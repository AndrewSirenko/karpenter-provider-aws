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

package instance

import (
	"math"

	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/samber/lo"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/aws/karpenter-provider-aws/pkg/providers/subnet"
)

const headroomTemperature = 0.25

func headroomWeight(freeIPs, neededIPs int64) float64 {
	h := math.Min(1, float64(freeIPs)/float64(max(neededIPs, 1)))
	return math.Exp((math.Max(h, 0) - 1) / headroomTemperature)
}

func weightOverridesByHeadroom(
	configs []ec2types.FleetLaunchTemplateConfigRequest,
	zonalSubnets map[string]*subnet.Subnet,
	instanceTypes []*cloudprovider.InstanceType,
	draw func() float64,
) ([]ec2types.FleetLaunchTemplateConfigRequest, int) {
	neededIPs := lo.SliceToMap(instanceTypes, func(it *cloudprovider.InstanceType) (string, int64) {
		return it.Name, it.Capacity.Pods().Value() + 1
	})
	kept := make([][]ec2types.FleetLaunchTemplateOverridesRequest, len(configs))
	bestConfig, bestOverride, bestWeight, dropped := -1, -1, -1.0, 0
	for ci, config := range configs {
		for oi, override := range config.Overrides {
			s, ok := zonalSubnets[lo.FromPtr(override.AvailabilityZone)]
			if !ok {
				kept[ci] = append(kept[ci], override)
				continue
			}
			w := headroomWeight(int64(s.AvailableIPAddressCount), neededIPs[string(override.InstanceType)])
			if w > bestWeight {
				bestConfig, bestOverride, bestWeight = ci, oi, w
			}
			if draw() < w {
				kept[ci] = append(kept[ci], override)
			} else {
				dropped++
			}
		}
	}
	if bestConfig >= 0 && lo.EveryBy(kept, func(o []ec2types.FleetLaunchTemplateOverridesRequest) bool { return len(o) == 0 }) {
		kept[bestConfig] = append(kept[bestConfig], configs[bestConfig].Overrides[bestOverride])
		dropped--
	}
	var out []ec2types.FleetLaunchTemplateConfigRequest
	for ci, config := range configs {
		if len(kept[ci]) == 0 {
			continue
		}
		config.Overrides = kept[ci]
		out = append(out, config)
	}
	return out, dropped
}
