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

package cloudprovider_test

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corecloudprovider "sigs.k8s.io/karpenter/pkg/cloudprovider"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/cloudprovider"
	"github.com/aws/karpenter-provider-aws/pkg/test"
)

type fakeVolumeAPI struct {
	mu        sync.Mutex
	volumes   map[string]ec2types.Volume
	mods      map[string]ec2types.VolumeModification
	modify    []*ec2.ModifyVolumeInput
	modifyErr error
}

func (f *fakeVolumeAPI) DescribeInstances(_ context.Context, in *ec2.DescribeInstancesInput, _ ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return &ec2.DescribeInstancesOutput{Reservations: []ec2types.Reservation{{Instances: []ec2types.Instance{{
		InstanceId: aws.String(in.InstanceIds[0]),
		BlockDeviceMappings: []ec2types.InstanceBlockDeviceMapping{
			{DeviceName: aws.String("/dev/xvda"), Ebs: &ec2types.EbsInstanceBlockDevice{VolumeId: aws.String("vol-root")}},
		},
	}}}}}, nil
}

func (f *fakeVolumeAPI) DescribeVolumes(_ context.Context, in *ec2.DescribeVolumesInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &ec2.DescribeVolumesOutput{}
	for _, id := range in.VolumeIds {
		out.Volumes = append(out.Volumes, f.volumes[id])
	}
	return out, nil
}

func (f *fakeVolumeAPI) DescribeVolumesModifications(_ context.Context, in *ec2.DescribeVolumesModificationsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVolumesModificationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &ec2.DescribeVolumesModificationsOutput{}
	for _, id := range in.VolumeIds {
		if m, ok := f.mods[id]; ok {
			out.VolumesModifications = append(out.VolumesModifications, m)
		}
	}
	return out, nil
}

func (f *fakeVolumeAPI) ModifyVolume(_ context.Context, in *ec2.ModifyVolumeInput, _ ...func(*ec2.Options)) (*ec2.ModifyVolumeOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modify = append(f.modify, in)
	if f.modifyErr != nil {
		return nil, f.modifyErr
	}
	v := f.volumes[*in.VolumeId]
	f.mods[*in.VolumeId] = ec2types.VolumeModification{
		VolumeId: in.VolumeId, ModificationState: ec2types.VolumeModificationStateModifying,
		TargetIops: lo.Ternary(in.Iops != nil, in.Iops, v.Iops), TargetThroughput: lo.Ternary(in.Throughput != nil, in.Throughput, v.Throughput),
		TargetSize: lo.Ternary(in.Size != nil, in.Size, v.Size),
	}
	return &ec2.ModifyVolumeOutput{}, nil
}

// complete finishes the pending modification and applies its targets to the volume.
func (f *fakeVolumeAPI) complete(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.mods[id]
	v := f.volumes[id]
	v.Iops, v.Throughput, v.Size = m.TargetIops, m.TargetThroughput, m.TargetSize
	f.volumes[id] = v
	m.ModificationState = ec2types.VolumeModificationStateCompleted
	f.mods[id] = m
}

var _ = Describe("In-place volume Update", func() {
	var volumeAPI *fakeVolumeAPI
	var cp *cloudprovider.CloudProvider
	var nc *v1.EC2NodeClass
	var claim *karpv1.NodeClaim

	BeforeEach(func() {
		volumeAPI = &fakeVolumeAPI{
			volumes: map[string]ec2types.Volume{"vol-root": {
				VolumeId: aws.String("vol-root"), VolumeType: ec2types.VolumeTypeGp3,
				Size: aws.Int32(20), Iops: aws.Int32(3000), Throughput: aws.Int32(125),
			}},
			mods: map[string]ec2types.VolumeModification{},
		}
		cp = cloudprovider.New(awsEnv.InstanceTypesProvider, awsEnv.InstanceProvider, recorder, env.Client, awsEnv.AMIProvider,
			awsEnv.SecurityGroupProvider, awsEnv.CapacityReservationProvider, awsEnv.PlacementGroupProvider, awsEnv.InstanceTypeStore,
			lo.ToPtr("test-ca-bundle"), cloudprovider.WithVolumeAPI(volumeAPI))
		nc = test.EC2NodeClass(v1.EC2NodeClass{Spec: v1.EC2NodeClassSpec{BlockDeviceMappings: []*v1.BlockDeviceMapping{{
			DeviceName: aws.String("/dev/xvda"), RootVolume: true,
			EBS: &v1.BlockDevice{VolumeType: aws.String("gp3"), VolumeSize: lo.ToPtr(resource.MustParse("20Gi")), IOPS: aws.Int64(3000), Throughput: aws.Int64(125)},
		}}}})
		ExpectApplied(ctx, env.Client, nc)
		claim = coretest.NodeClaim(karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			Spec:       karpv1.NodeClaimSpec{NodeClassRef: &karpv1.NodeClassReference{Group: "karpenter.k8s.aws", Kind: "EC2NodeClass", Name: nc.Name}},
			Status:     karpv1.NodeClaimStatus{ProviderID: "aws:///test-zone-1a/i-0123456789abcdef0"},
		})
	})

	It("is done without an override when the volume matches the EC2NodeClass", func() {
		done, err := cp.Update(ctx, claim, "op")
		Expect(err).ToNot(HaveOccurred())
		Expect(done).To(BeTrue())
		Expect(volumeAPI.modify).To(BeEmpty())
	})
	It("merges the override onto the EC2NodeClass and modifies the volume, growing size", func() {
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500,"size":"30Gi"}}}`
		done, err := cp.Update(ctx, claim, "op")
		Expect(err).ToNot(HaveOccurred())
		Expect(done).To(BeFalse())
		Expect(volumeAPI.modify).To(HaveLen(1))
		Expect(*volumeAPI.modify[0].Iops).To(BeNumerically("==", 6000))
		Expect(*volumeAPI.modify[0].Throughput).To(BeNumerically("==", 500))
		Expect(*volumeAPI.modify[0].Size).To(BeNumerically("==", 30))

		By("not modifying again while the same modification is running")
		done, err = cp.Update(ctx, claim, "op")
		Expect(err).ToNot(HaveOccurred())
		Expect(done).To(BeFalse())
		Expect(volumeAPI.modify).To(HaveLen(1))

		By("reporting done once EBS completes")
		volumeAPI.complete("vol-root")
		done, err = cp.Update(ctx, claim, "op")
		Expect(err).ToNot(HaveOccurred())
		Expect(done).To(BeTrue())
	})
	It("rejects an override that would shrink the volume", func() {
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvda":{"size":"10Gi"}}}`
		_, err := cp.Update(ctx, claim, "op")
		Expect(corecloudprovider.IsNodeUpdateFailedError(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("can't shrink"))
		Expect(volumeAPI.modify).To(BeEmpty())
	})
	It("keeps a grown volume when the override is removed, instead of shrinking to the EC2NodeClass size", func() {
		v := volumeAPI.volumes["vol-root"]
		v.Size = aws.Int32(30)
		volumeAPI.volumes["vol-root"] = v
		done, err := cp.Update(ctx, claim, "op")
		Expect(err).ToNot(HaveOccurred())
		Expect(done).To(BeTrue())
		Expect(volumeAPI.modify).To(BeEmpty())
	})
	It("rejects devices the EC2NodeClass doesn't map and unknown fields", func() {
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvdz":{"iops":6000}}}`
		_, err := cp.Update(ctx, claim, "op")
		Expect(corecloudprovider.IsNodeUpdateFailedError(err)).To(BeTrue())
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvda":{"kmsKeyID":"x"}}}`
		_, err = cp.Update(ctx, claim, "op")
		Expect(corecloudprovider.IsNodeUpdateFailedError(err)).To(BeTrue())
		Expect(volumeAPI.modify).To(BeEmpty())
	})
	It("throttles when a modification toward a different target is still running", func() {
		volumeAPI.mods["vol-root"] = ec2types.VolumeModification{VolumeId: aws.String("vol-root"), ModificationState: ec2types.VolumeModificationStateOptimizing,
			TargetIops: aws.Int32(4000), TargetThroughput: aws.Int32(125), TargetSize: aws.Int32(20)}
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvda":{"iops":6000}}}`
		_, err := cp.Update(ctx, claim, "op")
		Expect(corecloudprovider.IsNodeUpdateThrottledError(err)).To(BeTrue())
		Expect(volumeAPI.modify).To(BeEmpty())
	})
	It("waits until the time EBS gives when the volume's modification rate is exceeded", func() {
		volumeAPI.modifyErr = &smithy.GenericAPIError{Code: "VolumeModificationRateExceeded",
			Message: "You've reached the maximum modification rate per volume limit. Wait until 2099-01-01T00:00:00.000Z before you can issue the next modification request for this volume."}
		claim.Annotations[karpv1.NodeClassOverrideAnnotationKey] = `{"blockDeviceMappings":{"/dev/xvda":{"throughput":250}}}`
		_, err := cp.Update(ctx, claim, "op")
		var te *corecloudprovider.NodeUpdateThrottledError
		Expect(errors.As(err, &te)).To(BeTrue())
		Expect(te.RetryAfter).To(BeNumerically(">", 24*time.Hour))
	})
})
