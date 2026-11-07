/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// POC for designs/in-place-ebs-performance.md: apply EBS IOPS/throughput from the EC2NodeClass plus the
// karpenter.sh/nodeclass-override annotation to a running instance's volumes with ModifyVolume.
package cloudprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/log"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/aws/karpenter-provider-aws/pkg/utils"
)

// VolumeAPI is the subset of EC2 the in-place volume update needs.
type VolumeAPI interface {
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	DescribeVolumesModifications(context.Context, *ec2.DescribeVolumesModificationsInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesModificationsOutput, error)
	ModifyVolume(context.Context, *ec2.ModifyVolumeInput, ...func(*ec2.Options)) (*ec2.ModifyVolumeOutput, error)
}

// WithVolumeAPI enables in-place EBS performance updates.
func WithVolumeAPI(api VolumeAPI) Option {
	return func(c *CloudProvider) { c.volumeAPI = api }
}

type volumeTarget struct {
	IOPS       *int32             `json:"iops,omitempty"`
	Throughput *int32             `json:"throughput,omitempty"`
	Size       *resource.Quantity `json:"size,omitempty"`
	// sizeFromOverride is set when Size came from the per-NodeClaim override. EBS volumes can't shrink, so an
	// override below the current size is rejected, while a NodeClass size below it (e.g. after growth) is ignored.
	sizeFromOverride bool
}

func gib(q *resource.Quantity) int32 {
	return int32((q.Value() + (1 << 30) - 1) >> 30)
}

type nodeClassOverride struct {
	BlockDeviceMappings map[string]volumeTarget `json:"blockDeviceMappings"`
}

func rejected(format string, args ...any) error {
	return cloudprovider.NewNodeUpdateFailedError(true, fmt.Errorf(format, args...))
}

// Update brings the instance's EBS IOPS and throughput in line with its EC2NodeClass and any override.
// It returns done=true once every targeted volume matches and no modification is still running.
//
//nolint:gocyclo
func (c *CloudProvider) Update(ctx context.Context, nodeClaim *karpv1.NodeClaim, operationID string) (bool, error) {
	if c.volumeAPI == nil {
		return false, cloudprovider.NewNodeUpdateNotImplementedError()
	}
	nodeClass, err := c.resolveNodeClassFromNodeClaim(ctx, nodeClaim)
	if err != nil {
		return false, fmt.Errorf("resolving nodeclass, %w", err)
	}

	targets := map[string]volumeTarget{}
	for _, bdm := range nodeClass.Spec.BlockDeviceMappings {
		if bdm == nil || bdm.DeviceName == nil || bdm.EBS == nil {
			continue
		}
		t := volumeTarget{}
		if bdm.EBS.IOPS != nil {
			t.IOPS = aws.Int32(int32(*bdm.EBS.IOPS))
		}
		if bdm.EBS.Throughput != nil {
			t.Throughput = aws.Int32(int32(*bdm.EBS.Throughput))
		}
		if bdm.EBS.VolumeSize != nil {
			t.Size = bdm.EBS.VolumeSize
		}
		targets[*bdm.DeviceName] = t
	}
	if raw, ok := nodeClaim.Annotations[karpv1.NodeClassOverrideAnnotationKey]; ok {
		o := nodeClassOverride{}
		dec := json.NewDecoder(bytes.NewBufferString(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&o); err != nil {
			return false, rejected("parsing %s, %w", karpv1.NodeClassOverrideAnnotationKey, err)
		}
		for dev, ot := range o.BlockDeviceMappings {
			base, ok := targets[dev]
			if !ok {
				return false, rejected("device %q is not in EC2NodeClass %q blockDeviceMappings", dev, nodeClass.Name)
			}
			if ot.IOPS != nil {
				base.IOPS = ot.IOPS
			}
			if ot.Throughput != nil {
				base.Throughput = ot.Throughput
			}
			if ot.Size != nil {
				base.Size = ot.Size
				base.sizeFromOverride = true
			}
			targets[dev] = base
		}
	}

	id, err := utils.ParseInstanceID(nodeClaim.Status.ProviderID)
	if err != nil {
		return false, fmt.Errorf("getting instance ID, %w", err)
	}
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("id", id, "operationID", operationID))
	instances, err := c.volumeAPI.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{id}})
	if err != nil {
		return false, fmt.Errorf("describing instance, %w", err)
	}
	volumeByDevice := map[string]string{}
	for _, r := range instances.Reservations {
		for _, i := range r.Instances {
			for _, m := range i.BlockDeviceMappings {
				if m.DeviceName != nil && m.Ebs != nil && m.Ebs.VolumeId != nil {
					volumeByDevice[*m.DeviceName] = *m.Ebs.VolumeId
				}
			}
		}
	}

	devices := make([]string, 0, len(targets))
	for dev, t := range targets {
		if t.IOPS == nil && t.Throughput == nil && t.Size == nil {
			continue
		}
		if _, ok := volumeByDevice[dev]; !ok {
			return false, cloudprovider.NewNodeUpdateFailedError(false, fmt.Errorf("device %q has no EBS volume attached to %s", dev, id))
		}
		devices = append(devices, dev)
	}
	sort.Strings(devices)
	if len(devices) == 0 {
		return true, nil
	}
	volumeIDs := make([]string, 0, len(devices))
	for _, dev := range devices {
		volumeIDs = append(volumeIDs, volumeByDevice[dev])
	}

	vols, err := c.volumeAPI.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{VolumeIds: volumeIDs})
	if err != nil {
		return false, fmt.Errorf("describing volumes, %w", err)
	}
	volumes := map[string]ec2types.Volume{}
	for _, v := range vols.Volumes {
		volumes[aws.ToString(v.VolumeId)] = v
	}
	mods := map[string]ec2types.VolumeModification{}
	modsOut, err := c.volumeAPI.DescribeVolumesModifications(ctx, &ec2.DescribeVolumesModificationsInput{VolumeIds: volumeIDs})
	if err != nil {
		var ae smithy.APIError
		if !errors.As(err, &ae) || ae.ErrorCode() != "InvalidVolumeModification.NotFound" {
			return false, fmt.Errorf("describing volume modifications, %w", err)
		}
	} else {
		for _, m := range modsOut.VolumesModifications {
			id := aws.ToString(m.VolumeId)
			if cur, ok := mods[id]; !ok || aws.ToTime(m.StartTime).After(aws.ToTime(cur.StartTime)) {
				mods[id] = m
			}
		}
	}

	done := true
	var progress []string
	for _, dev := range devices {
		volID := volumeByDevice[dev]
		v, ok := volumes[volID]
		if !ok {
			return false, cloudprovider.NewNodeUpdateFailedError(false, fmt.Errorf("volume %s for device %q not found", volID, dev))
		}
		t := targets[dev]
		wantIOPS := aws.ToInt32(v.Iops)
		if t.IOPS != nil {
			wantIOPS = *t.IOPS
		}
		wantThroughput := aws.ToInt32(v.Throughput)
		if t.Throughput != nil {
			if v.VolumeType != ec2types.VolumeTypeGp3 {
				return false, rejected("device %q is %s, throughput is only configurable on gp3", dev, v.VolumeType)
			}
			wantThroughput = *t.Throughput
		}

		wantSize := aws.ToInt32(v.Size)
		if t.Size != nil {
			switch want := gib(t.Size); {
			case want > wantSize:
				wantSize = want
			case want < wantSize && t.sizeFromOverride:
				return false, rejected("device %q is %dGiB, EBS volumes can't shrink to %dGiB", dev, wantSize, want)
			}
		}

		if m, ok := mods[volID]; ok && (m.ModificationState == ec2types.VolumeModificationStateModifying || m.ModificationState == ec2types.VolumeModificationStateOptimizing) {
			if aws.ToInt32(m.TargetIops) == wantIOPS && aws.ToInt32(m.TargetSize) == wantSize && (v.VolumeType != ec2types.VolumeTypeGp3 || aws.ToInt32(m.TargetThroughput) == wantThroughput) {
				done = false
				progress = append(progress, fmt.Sprintf("%s %s %s %d%%", dev, volID, m.ModificationState, aws.ToInt64(m.Progress)))
				continue
			}
			return false, cloudprovider.NewNodeUpdateThrottledError(time.Minute, fmt.Errorf("volume %s has a modification in progress toward a different target", volID))
		}
		if aws.ToInt32(v.Iops) == wantIOPS && aws.ToInt32(v.Size) == wantSize && (v.VolumeType != ec2types.VolumeTypeGp3 || aws.ToInt32(v.Throughput) == wantThroughput) {
			continue
		}

		in := &ec2.ModifyVolumeInput{VolumeId: aws.String(volID), Iops: aws.Int32(wantIOPS)}
		if wantSize != aws.ToInt32(v.Size) {
			in.Size = aws.Int32(wantSize)
		}
		if v.VolumeType == ec2types.VolumeTypeGp3 {
			in.Throughput = aws.Int32(wantThroughput)
		}
		log.FromContext(ctx).Info("modifying volume", "device", dev, "volume", volID,
			"fromIOPS", aws.ToInt32(v.Iops), "toIOPS", wantIOPS, "fromThroughput", aws.ToInt32(v.Throughput), "toThroughput", wantThroughput,
			"fromSizeGiB", aws.ToInt32(v.Size), "toSizeGiB", wantSize)
		if _, err := c.volumeAPI.ModifyVolume(ctx, in); err != nil {
			var ae smithy.APIError
			if errors.As(err, &ae) {
				switch {
				case ae.ErrorCode() == "VolumeModificationRateExceeded":
					// EBS allows a few modifications per volume per rolling 24h and says when the next one is allowed,
					// e.g. "Wait until 2026-11-08T04:59:26.792Z before you can issue the next modification request".
					return false, cloudprovider.NewNodeUpdateThrottledError(retryAfterFromMessage(ae.ErrorMessage(), time.Now(), 30*time.Minute), err)
				case ae.ErrorCode() == "IncorrectModificationState":
					return false, cloudprovider.NewNodeUpdateThrottledError(time.Minute, err)
				case strings.HasPrefix(ae.ErrorCode(), "InvalidParameter"):
					return false, rejected("modifying volume %s, %w", volID, err)
				case ae.ErrorCode() == "UnauthorizedOperation":
					return false, cloudprovider.NewNodeUpdateFailedError(false, err)
				}
			}
			return false, fmt.Errorf("modifying volume %s, %w", volID, err)
		}
		done = false
		progress = append(progress, fmt.Sprintf("%s %s modify requested", dev, volID))
	}
	if !done {
		log.FromContext(ctx).Info("in-place volume update in progress", "volumes", strings.Join(progress, "; "))
	}
	return done, nil
}

var waitUntil = regexp.MustCompile(`Wait until (\S+?Z)`)

func retryAfterFromMessage(msg string, now time.Time, fallback time.Duration) time.Duration {
	m := waitUntil.FindStringSubmatch(msg)
	if m == nil {
		return fallback
	}
	until, err := time.Parse(time.RFC3339Nano, m[1])
	if err != nil || !until.After(now) {
		return fallback
	}
	return until.Sub(now) + 5*time.Second
}
