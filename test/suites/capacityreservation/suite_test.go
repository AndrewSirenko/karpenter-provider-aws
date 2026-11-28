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

package capacityreservation_test

import (
	"testing"

	"github.com/aws/karpenter-provider-aws/test/pkg/environment/aws"
	"github.com/aws/karpenter-provider-aws/test/suites/capacityreservation"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCapacityReservation(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		capacityreservation.Env = aws.NewEnvironment(t)
	})
	AfterSuite(func() {
		capacityreservation.Env.Stop()
	})
	RunSpecs(t, "CapacityReservation")
}

var _ = BeforeEach(func() {
	capacityreservation.Env.BeforeEach()
	capacityreservation.NodeClass = capacityreservation.Env.DefaultEC2NodeClass()
	capacityreservation.NodePool = capacityreservation.Env.DefaultNodePool(capacityreservation.NodeClass)
})
var _ = AfterEach(func() { capacityreservation.Env.Cleanup() })
var _ = AfterEach(func() { capacityreservation.Env.AfterEach() })

var _ = capacityreservation.RegisterTests()
