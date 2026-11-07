# In-place NodeClaim override POC

The e2e passes on a real EKS cluster with modified Karpenter core and provider-aws. One annotation on one NodeClaim changed that node's root EBS volume from 3000 IOPS / 125 MiB/s to 6000 / 500 in place. The instance, boot ID and pod were unchanged, the node was never cordoned, and no other node was touched. Removing the annotation reverted the volume to the EC2NodeClass values.

## Proof

- `e2e-proof.md`: full e2e (`04-e2e.sh`), 10/10 PASS, plus the revert follow-up (steps 4 and 5).
- `e2e-mutation-proof.md`: same core, provider image built without `WithVolumeAPI`. The override is `OverrideRejected` and the volume stays 3000/125 (`06-mutation.sh`).
- `unit-proof.md`: core `nodeclaim.inplace` Ginkgo suite, 7/7. Dropping the applied-override marker fails 2.

## Timeline (vol-06834de2b44ed41f1, i-053cf8b312a3c128c, c6a.large, ca-central-1b)

| UTC | Event |
|---|---|
| ~02:39 | Override annotated. ModifyVolume to 6000/500 issued on the first reconcile |
| 02:39:13 | DescribeVolumes already shows 6000/500 (`optimizing`) |
| 02:44:09 | Modification `completed`, and `Updating=False/UpdateSucceeded` at 02:44:10 |
| 02:44:22 | Invalid override (`/dev/xvdz`) set. `OverrideRejected`, nothing changed |
| 02:44:25 | Override removed. ModifyVolume back to 3000/125 accepted, no throttling |
| 02:49:29 | Revert `completed`, `UpdateSucceeded`, applied marker cleared |

## What was built

**kubernetes-sigs/karpenter** (`../poc-karpenter`, branch `inplace-poc`, off main dbc34904):

- `CloudProvider.Update(ctx, nodeClaim, operationID) (done bool, err error)`, with `NodeUpdateNotImplementedError`, `NodeUpdateFailedError{Rejected}` and `NodeUpdateThrottledError{RetryAfter}`.
- Annotations `karpenter.sh/nodeclass-override` (user), `karpenter.sh/nodeclass-override-applied` and `karpenter.sh/update-operation-id` (controller).
- Condition `Updating` with reasons `UpdateRequested`, `UpdateIssued`, `UpdateThrottled`, `UpdateSucceeded`, `UpdateFailed` and `OverrideRejected`.
- A new `nodeclaim.inplace` controller. It waits for `Initialized` and skips `Rebooting` nodes. It hashes the override to get an operation ID, polls the provider every 15s, and records the applied hash when done.
- Consolidation and emptiness skip NodeClaims that have an override.
- The fake and kwok providers return not-implemented. The fake has `UpdateFunc` for tests.

**aws/karpenter-provider-aws** (`../poc-provider-aws`, branch `inplace-poc`, off main cd27e6d7a, `replace sigs.k8s.io/karpenter => ../poc-karpenter`):

- `pkg/cloudprovider/inplace.go`, `CloudProvider.Update`:
  - The target is the EC2NodeClass `blockDeviceMappings` iops/throughput, with the override merged on top, keyed by device name. Unknown fields or devices are rejected.
  - Volumes are found with DescribeInstances, then DescribeVolumes by ID.
  - DescribeVolumesModifications is checked first, so retries never double-modify.
  - It calls ModifyVolume, and reports done only at `completed`.
  - Error mapping: rate-limit errors become throttled, `InvalidParameter*` becomes rejected, and `UnauthorizedOperation` becomes failed.
- `WithVolumeAPI(op.EC2API)` option in `cmd/controller/main.go`.
- IAM: inline policy `InPlaceVolumePOC` (ModifyVolume, DescribeVolumes, DescribeVolumesModifications) on `inplace-poc-karpenter`.

## Findings

1. **No 6-hour cooldown in practice.** A second ModifyVolume on the same volume was accepted 16 seconds after the first completed. The RFCs and issue drafts now say EBS "can rate-limit repeated modifications" and that the provider retries on the error.
2. **Performance lands before `completed`.** DescribeVolumes showed the new IOPS and throughput about 20s after the call, while the modification was still `optimizing` at 0%. `completed` came about 5 minutes later. Reporting done at `completed` is conservative. Users get the performance sooner than the condition says.
3. **The root volume works** (`/dev/xvda`, `rootVolume: true`). A data volume should behave the same, but it wasn't tested.
4. **Gotcha.** The sandbox sets `AWS_REGION=us-west-2`, which beats `AWS_DEFAULT_REGION`. The first CFN stack and ECR repo landed in us-west-2, and the region-scoped controller policies denied `DescribeInstanceTypes` in ca-central-1. Fixed by `05-fix-iam-region.sh`, and all scripts now export `AWS_REGION`.

## Not done (POC shortcuts)

- No drift-driven `Update` (`GetDriftStatus`, split hash, `blockDeviceDriftPolicy`). Only overrides.
- No feature gate, events or metrics. Status is the condition plus logs.
- Revert targets only fields the EC2NodeClass sets explicitly.
- No provider-aws unit tests.
- `cmd/controller/kodata` symlinks were replaced locally, uncommitted, so ko works inside a git worktree.

## Scripts

`01-infra.sh` (CFN and eksctl), `02-image.sh` (ko to ECR), `03-deploy.sh` (IAM and helm), `04-e2e.sh`, `05-fix-iam-region.sh`, `06-mutation.sh`. Go uses `~/sdk/go1.26.10`, `GOTOOLCHAIN=local GOPROXY=direct GOSUMDB=off GIT_CONFIG_GLOBAL=/dev/null`.
