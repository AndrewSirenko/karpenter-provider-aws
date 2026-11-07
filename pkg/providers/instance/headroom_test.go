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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/aws/karpenter-provider-aws/pkg/providers/subnet"
)

var _ = Describe("weightOverridesByHeadroom", func() {
	var instanceTypes []*cloudprovider.InstanceType
	override := func(it, zone string) ec2types.FleetLaunchTemplateOverridesRequest {
		return ec2types.FleetLaunchTemplateOverridesRequest{InstanceType: ec2types.InstanceType(it), AvailabilityZone: lo.ToPtr(zone)}
	}
	config := func(overrides ...ec2types.FleetLaunchTemplateOverridesRequest) ec2types.FleetLaunchTemplateConfigRequest {
		return ec2types.FleetLaunchTemplateConfigRequest{Overrides: overrides}
	}
	zones := func(configs []ec2types.FleetLaunchTemplateConfigRequest) []string {
		return lo.FlatMap(configs, func(c ec2types.FleetLaunchTemplateConfigRequest, _ int) []string {
			return lo.Map(c.Overrides, func(o ec2types.FleetLaunchTemplateOverridesRequest, _ int) string {
				return lo.FromPtr(o.AvailabilityZone)
			})
		})
	}
	half := func() float64 { return 0.5 }
	BeforeEach(func() {
		instanceTypes = []*cloudprovider.InstanceType{
			{Name: "m5.large", Capacity: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("29")}},
			{Name: "m5.xlarge", Capacity: corev1.ResourceList{corev1.ResourcePods: resource.MustParse("58")}},
		}
	})
	It("should weight by free IPs per IPs needed for one node", func() {
		Expect(headroomWeight(100, 30)).To(Equal(1.0))
		Expect(headroomWeight(15, 30)).To(BeNumerically("~", math.Exp(-2), 1e-9))
		Expect(headroomWeight(0, 30)).To(BeNumerically("~", math.Exp(-4), 1e-9))
	})
	It("should drop overrides in a zone without room for a node and keep the zone with room", func() {
		subnets := map[string]*subnet.Subnet{
			"zone-a": {ID: "subnet-a", Zone: "zone-a", AvailableIPAddressCount: 6},
			"zone-b": {ID: "subnet-b", Zone: "zone-b", AvailableIPAddressCount: 1000},
		}
		out, dropped := weightOverridesByHeadroom([]ec2types.FleetLaunchTemplateConfigRequest{
			config(override("m5.large", "zone-a"), override("m5.large", "zone-b"), override("m5.xlarge", "zone-a"), override("m5.xlarge", "zone-b")),
		}, subnets, instanceTypes, half)
		Expect(zones(out)).To(ConsistOf("zone-b", "zone-b"))
		Expect(dropped).To(Equal(2))
	})
	It("should keep every override when all zones have room", func() {
		subnets := map[string]*subnet.Subnet{
			"zone-a": {ID: "subnet-a", Zone: "zone-a", AvailableIPAddressCount: 500},
			"zone-b": {ID: "subnet-b", Zone: "zone-b", AvailableIPAddressCount: 500},
		}
		out, dropped := weightOverridesByHeadroom([]ec2types.FleetLaunchTemplateConfigRequest{
			config(override("m5.large", "zone-a"), override("m5.xlarge", "zone-b")),
		}, subnets, instanceTypes, func() float64 { return 0.999 })
		Expect(zones(out)).To(ConsistOf("zone-a", "zone-b"))
		Expect(dropped).To(BeZero())
	})
	It("should keep the single best override across every launch template when all would be dropped", func() {
		subnets := map[string]*subnet.Subnet{
			"zone-a": {ID: "subnet-a", Zone: "zone-a", AvailableIPAddressCount: 0},
			"zone-b": {ID: "subnet-b", Zone: "zone-b", AvailableIPAddressCount: 20},
		}
		out, dropped := weightOverridesByHeadroom([]ec2types.FleetLaunchTemplateConfigRequest{
			config(override("m5.large", "zone-a"), override("m5.xlarge", "zone-a")),
			config(override("m5.large", "zone-b"), override("m5.xlarge", "zone-b")),
		}, subnets, instanceTypes, func() float64 { return 1 })
		Expect(out).To(HaveLen(1))
		Expect(out[0].Overrides).To(HaveLen(1))
		Expect(out[0].Overrides[0].InstanceType).To(Equal(ec2types.InstanceType("m5.large")))
		Expect(zones(out)).To(ConsistOf("zone-b"))
		Expect(dropped).To(Equal(3))
	})
	It("should keep overrides in zones it has no subnet for", func() {
		out, dropped := weightOverridesByHeadroom([]ec2types.FleetLaunchTemplateConfigRequest{
			config(override("m5.large", "zone-c")),
		}, map[string]*subnet.Subnet{}, instanceTypes, func() float64 { return 1 })
		Expect(zones(out)).To(ConsistOf("zone-c"))
		Expect(dropped).To(BeZero())
	})
})
