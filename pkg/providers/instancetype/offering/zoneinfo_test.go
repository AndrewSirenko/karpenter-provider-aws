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

package offering_test

import (
	"strings"
	"testing"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
	"github.com/aws/karpenter-provider-aws/pkg/providers/instancetype/offering"
)

func TestValidateZoneInfo(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      []v1.ZoneInfo
		wantErr string
	}{
		{name: "grouped by zone", in: []v1.ZoneInfo{
			{Zone: "a", ZoneID: "z1", SubnetIDs: []string{"subnet-1", "subnet-2"}},
			{Zone: "b", ZoneID: "z2", SubnetIDs: []string{"subnet-3"}},
		}},
		{name: "empty", in: nil},
		{name: "missing SubnetIDs", in: []v1.ZoneInfo{{Zone: "a", ZoneID: "z1"}}, wantErr: `zone "a" has no SubnetIDs`},
		{
			name: "one entry per subnet without SubnetIDs",
			in: []v1.ZoneInfo{
				{Zone: "a", ZoneID: "z1"},
				{Zone: "a", ZoneID: "z1"},
				{Zone: "b", ZoneID: "z2"},
			},
			wantErr: `zone "a" appears more than once`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := offering.ValidateZoneInfo(tc.in)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestEC2NodeClassZoneInfoSatisfiesContract(t *testing.T) {
	nc := &v1.EC2NodeClass{Status: v1.EC2NodeClassStatus{Subnets: []v1.Subnet{
		{ID: "subnet-1", Zone: "a", ZoneID: "z1"},
		{ID: "subnet-2", Zone: "a", ZoneID: "z1"},
		{ID: "subnet-3", Zone: "b", ZoneID: "z2"},
	}}}
	if err := offering.ValidateZoneInfo(nc.ZoneInfo()); err != nil {
		t.Fatalf("EC2NodeClass.ZoneInfo() violates the offering contract: %v", err)
	}
}
