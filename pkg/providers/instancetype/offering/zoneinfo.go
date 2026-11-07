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
	"errors"
	"fmt"

	v1 "github.com/aws/karpenter-provider-aws/pkg/apis/v1"
)

func ValidateZoneInfo(zoneInfo []v1.ZoneInfo) error {
	var errs []error
	seen := map[string]bool{}
	for _, zi := range zoneInfo {
		if seen[zi.Zone] {
			errs = append(errs, fmt.Errorf("zone %q appears more than once in ZoneInfo, group its subnets into one entry", zi.Zone))
			continue
		}
		seen[zi.Zone] = true
		if len(zi.SubnetIDs) == 0 {
			errs = append(errs, fmt.Errorf("zone %q has no SubnetIDs in ZoneInfo, so subnets that run out of IP addresses can never make the zone unavailable", zi.Zone))
		}
	}
	return errors.Join(errs...)
}
