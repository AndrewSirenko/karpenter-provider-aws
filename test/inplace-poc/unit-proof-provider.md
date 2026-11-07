# provider-aws in-place Update unit tests (2026-11-07T04:47:42Z)

```
$ go test ./pkg/cloudprovider/ -count=1 -v -args -ginkgo.focus='In-place volume Update'  # mutation: shrink guard removed
  [FAIL] In-place volume Update [It] rejects an override that would shrink the volume
FAIL! -- 5 Passed | 1 Failed | 0 Pending | 141 Skipped
Ran 6 of 147 Specs in 10.510 seconds

$ go test ./pkg/cloudprovider/ -count=1 -v -args -ginkgo.focus='In-place volume Update'  # restored
Ran 6 of 147 Specs in 10.513 seconds
SUCCESS! -- 6 Passed | 0 Failed | 0 Pending | 141 Skipped
ok  	github.com/aws/karpenter-provider-aws/pkg/cloudprovider	10.698s
```
