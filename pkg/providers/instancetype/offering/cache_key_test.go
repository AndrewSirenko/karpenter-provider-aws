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

package offering

import (
	"fmt"
	"hash/maphash"
	"testing"

	"github.com/mitchellh/hashstructure/v2"
	corev1 "k8s.io/api/core/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

func instanceTypeWithCapacityTypes(name string, capacityTypes ...string) *cloudprovider.InstanceType {
	return &cloudprovider.InstanceType{
		Name: name,
		Requirements: scheduling.NewRequirements(
			scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "test-zone-1a", "test-zone-1b"),
			scheduling.NewRequirement(karpv1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, capacityTypes...),
		),
	}
}

func TestCacheKeyFromInstanceTypeOtherCapacityTypes(t *testing.T) {
	b := &cacheKeyBuilder{baseSuffix: "base", capacityTypesHashes: map[capacityTypesKey]uint64{}, otherCapacityTypesHashes: map[uint64]uint64{}, seed: maphash.MakeSeed()}
	for _, capacityTypes := range [][]string{
		{karpv1.CapacityTypeOnDemand, "on-demand-alt"},
		{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeSpot, karpv1.CapacityTypeReserved},
	} {
		it := instanceTypeWithCapacityTypes("m5.large", capacityTypes...)
		for range 2 {
			zonesHash, _ := hashstructure.Hash(it.Requirements.Get(corev1.LabelTopologyZone).Values(), hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
			capacityTypesHash, _ := hashstructure.Hash(it.Requirements.Get(karpv1.CapacityTypeLabelKey).Values(), hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
			want := fmt.Sprintf("%s-%016x-%016x-%s", it.Name, zonesHash, capacityTypesHash, "base")
			if got := b.cacheKeyFromInstanceType(it); got != want {
				t.Fatalf("cache key for %v = %s, want %s", capacityTypes, got, want)
			}
		}
	}
	if len(b.otherCapacityTypesHashes) != 2 {
		t.Fatalf("hashed %d capacity type combinations, want 2", len(b.otherCapacityTypesHashes))
	}
}

func BenchmarkCacheKeyFromInstanceType(b *testing.B) {
	for _, capacityTypes := range [][]string{
		{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeSpot},
		{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeSpot, "on-demand-alt"},
	} {
		b.Run(fmt.Sprint(len(capacityTypes)), func(b *testing.B) {
			builder := &cacheKeyBuilder{baseSuffix: "base", capacityTypesHashes: map[capacityTypesKey]uint64{}, otherCapacityTypesHashes: map[uint64]uint64{}, seed: maphash.MakeSeed()}
			builder.capacityTypesHashes[capacityTypesKey{hasOnDemand: true, hasSpot: true}], _ = hashstructure.Hash([]string{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeSpot}, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
			its := make([]*cloudprovider.InstanceType, 800)
			for i := range its {
				its[i] = instanceTypeWithCapacityTypes(fmt.Sprintf("m5.%d", i), capacityTypes...)
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, it := range its {
					builder.cacheKeyFromInstanceType(it)
				}
			}
		})
	}
}
