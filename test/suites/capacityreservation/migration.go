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

package capacityreservation

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/test/pkg/environment/aws"
)

var Env *aws.Environment
var NodeClass *v1.EC2NodeClass
var NodePool *karpv1.NodePool

const migrationTimeout = 8 * time.Minute

func RegisterTests() bool {
	return Describe("Capacity Reservation Migration", Label("needs:capacity-reservation"), func() {
		var selectorTag string
		var reservations []string
		BeforeEach(func() {
			gates, _ := lo.Find(Env.ExpectSettings(), func(v corev1.EnvVar) bool { return v.Name == "AWS_FEATURE_GATES" })
			Env.ExpectSettingsOverridden(corev1.EnvVar{Name: "AWS_FEATURE_GATES", Value: strings.Trim(gates.Value+",ReservedCapacityMigration=true", ",")})
			// Select reservations by a tag no reservation has yet, so nodes launch on-demand with preference none and a
			// reservation only becomes eligible once the test tags it.
			selectorTag = fmt.Sprintf("odcr-migration-%s", uuid.NewString()[:8])
			NodeClass.Spec.CapacityReservationSelectorTerms = []v1.CapacityReservationSelectorTerm{{Tags: map[string]string{"karpenter-e2e-selector": selectorTag}}}
			NodePool.Spec.Template.Spec.Requirements = []karpv1.NodeSelectorRequirementWithMinValues{
				{Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand, karpv1.CapacityTypeReserved}},
				{Key: corev1.LabelOSStable, Operator: corev1.NodeSelectorOpIn, Values: []string{string(corev1.Linux)}},
			}
			reservations = nil
		})
		AfterEach(func() {
			for _, id := range reservations {
				_, _ = Env.EC2API.CancelCapacityReservation(Env.Context, &ec2.CancelCapacityReservationInput{CapacityReservationId: lo.ToPtr(id)})
			}
		})
		selectReservation := func(id string) {
			GinkgoHelper()
			_, err := Env.EC2API.CreateTags(Env.Context, &ec2.CreateTagsInput{
				Resources: []string{id},
				Tags:      []ec2types.Tag{{Key: lo.ToPtr("karpenter-e2e-selector"), Value: lo.ToPtr(selectorTag)}},
			})
			Expect(err).ToNot(HaveOccurred())
		}
		createReservation := func(instanceType, zone string) string {
			GinkgoHelper()
			out, err := Env.EC2API.CreateCapacityReservation(Env.Context, &ec2.CreateCapacityReservationInput{
				InstanceCount:         lo.ToPtr[int32](1),
				InstanceType:          lo.ToPtr(instanceType),
				InstancePlatform:      ec2types.CapacityReservationInstancePlatformLinuxUnix,
				AvailabilityZone:      lo.ToPtr(zone),
				InstanceMatchCriteria: ec2types.InstanceMatchCriteriaTargeted,
			})
			Expect(err).ToNot(HaveOccurred())
			reservations = append(reservations, *out.CapacityReservation.CapacityReservationId)
			return *out.CapacityReservation.CapacityReservationId
		}
		// launchOnDemand provisions one on-demand node for a pod pinned to the reservation's instance type and zone.
		launchOnDemand := func(cr aws.CapacityReservation, requirements ...corev1.NodeSelectorRequirement) (*corev1.Node, *corev1.Pod) {
			GinkgoHelper()
			pod := coretest.Pod(coretest.PodOptions{NodeRequirements: append(requirements,
				corev1.NodeSelectorRequirement{Key: corev1.LabelInstanceTypeStable, Operator: corev1.NodeSelectorOpIn, Values: []string{cr.InstanceType}},
				corev1.NodeSelectorRequirement{Key: corev1.LabelTopologyZone, Operator: corev1.NodeSelectorOpIn, Values: []string{cr.Zone}},
			)})
			Env.ExpectCreated(NodePool, NodeClass, pod)
			Env.EventuallyExpectHealthy(pod)
			node := Env.ExpectCreatedNodeCount("==", 1)[0]
			Expect(node.Labels).To(HaveKeyWithValue(karpv1.CapacityTypeLabelKey, karpv1.CapacityTypeOnDemand))
			instance := Env.GetInstance(node.Name)
			Expect(instance.CapacityReservationSpecification.CapacityReservationPreference).To(Equal(ec2types.CapacityReservationPreferenceNone))
			return node, pod
		}
		eventuallyReservedIn := func(node *corev1.Node, id string) {
			GinkgoHelper()
			Eventually(func(g Gomega) {
				n := &corev1.Node{}
				g.Expect(Env.Client.Get(Env.Context, client.ObjectKeyFromObject(node), n)).To(Succeed())
				g.Expect(n.Labels).To(HaveKeyWithValue(karpv1.CapacityTypeLabelKey, karpv1.CapacityTypeReserved))
				g.Expect(n.Labels).To(HaveKeyWithValue(v1.LabelCapacityReservationID, id))
			}).WithTimeout(migrationTimeout).WithPolling(10 * time.Second).Should(Succeed())
		}
		expectSameNodeAndPod := func(node *corev1.Node, pod *corev1.Pod) {
			GinkgoHelper()
			n := &corev1.Node{}
			Expect(Env.Client.Get(Env.Context, client.ObjectKeyFromObject(node), n)).To(Succeed())
			Expect(n.UID).To(Equal(node.UID))
			Expect(n.Status.NodeInfo.BootID).To(Equal(node.Status.NodeInfo.BootID))
			p := &corev1.Pod{}
			Expect(Env.Client.Get(Env.Context, client.ObjectKeyFromObject(pod), p)).To(Succeed())
			Expect(p.UID).To(Equal(pod.UID))
			Expect(p.Spec.NodeName).To(Equal(node.Name))
			Env.ExpectNodeClaimCount("==", 1)
		}

		It("should move a running on-demand node into a reservation without replacing it", func() {
			cr := Env.ExpectCapacityReservationCreated(1)
			reservations = append(reservations, cr.ID)
			node, pod := launchOnDemand(cr)

			selectReservation(cr.ID)
			eventuallyReservedIn(node, cr.ID)
			expectSameNodeAndPod(node, pod)
			instance := Env.GetInstance(node.Name)
			Expect(lo.FromPtr(instance.CapacityReservationId)).To(Equal(cr.ID))
			// The same shape EC2 reports for a reserved launch, so older releases still see the node as reserved.
			Expect(instance.CapacityReservationSpecification.CapacityReservationPreference).To(Equal(ec2types.CapacityReservationPreferenceCapacityReservationsOnly))
			Expect(lo.FromPtr(instance.CapacityReservationSpecification.CapacityReservationTarget.CapacityReservationId)).To(Equal(cr.ID))
		})
		It("should move a node into a new reservation after its reservation is cancelled", func() {
			cr := Env.ExpectCapacityReservationCreated(1)
			reservations = append(reservations, cr.ID)
			node, pod := launchOnDemand(cr)
			selectReservation(cr.ID)
			eventuallyReservedIn(node, cr.ID)

			next := createReservation(cr.InstanceType, cr.Zone)
			selectReservation(next)
			_, err := Env.EC2API.CancelCapacityReservation(Env.Context, &ec2.CancelCapacityReservationInput{CapacityReservationId: lo.ToPtr(cr.ID)})
			Expect(err).ToNot(HaveOccurred())
			eventuallyReservedIn(node, next)
			expectSameNodeAndPod(node, pod)
			Expect(lo.FromPtr(Env.GetInstance(node.Name).CapacityReservationId)).To(Equal(next))
		})
		It("should not move a node whose pods require on-demand", func() {
			cr := Env.ExpectCapacityReservationCreated(1)
			reservations = append(reservations, cr.ID)
			node, _ := launchOnDemand(cr, corev1.NodeSelectorRequirement{
				Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand},
			})
			selectReservation(cr.ID)
			Eventually(func(g Gomega) {
				nc := &v1.EC2NodeClass{}
				g.Expect(Env.Client.Get(Env.Context, client.ObjectKeyFromObject(NodeClass), nc)).To(Succeed())
				g.Expect(lo.Map(nc.Status.CapacityReservations, func(r v1.CapacityReservation, _ int) string { return r.ID })).To(ContainElement(cr.ID))
			}).WithTimeout(3 * time.Minute).Should(Succeed())
			Consistently(func(g Gomega) {
				instance := Env.GetInstance(node.Name)
				g.Expect(instance.CapacityReservationId).To(BeNil())
				g.Expect(instance.CapacityReservationSpecification.CapacityReservationTarget).To(BeNil())
			}).WithTimeout(3 * time.Minute).WithPolling(20 * time.Second).Should(Succeed())
		})
	})
}

var _ = Describe("Open capacity reservation matching", func() {
	It("should launch on-demand nodes with EC2's default open matching when the EC2NodeClass selects no reservations", func() {
		NodeClass.Spec.CapacityReservationSelectorTerms = nil
		NodePool.Spec.Template.Spec.Requirements = []karpv1.NodeSelectorRequirementWithMinValues{
			{Key: karpv1.CapacityTypeLabelKey, Operator: corev1.NodeSelectorOpIn, Values: []string{karpv1.CapacityTypeOnDemand}},
			{Key: corev1.LabelOSStable, Operator: corev1.NodeSelectorOpIn, Values: []string{string(corev1.Linux)}},
		}
		pod := coretest.Pod()
		Env.ExpectCreated(NodePool, NodeClass, pod)
		Env.EventuallyExpectHealthy(pod)
		node := Env.ExpectCreatedNodeCount("==", 1)[0]
		Expect(Env.GetInstance(node.Name).CapacityReservationSpecification.CapacityReservationPreference).To(Equal(ec2types.CapacityReservationPreferenceOpen))
	})
})
