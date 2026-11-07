# In-Place EBS Volume Performance

Change IOPS and throughput on a running node's EBS volumes with `ModifyVolume`, for EC2NodeClass changes and for one-off per-node overrides, without draining, rebooting or replacing the node.

This is the AWS provider half of [In-Place Drainless Drift Remediation](https://github.com/AndrewSirenko/karpenter/pull/30) (prototype on a fork). That RFC adds the `Update` action to #3460's `GetDriftStatus`, a `CloudProvider.Update` method, and the `karpenter.sh/nodeclass-override` NodeClaim annotation. It sits next to [In-Place AMI Upgrades](https://github.com/aws/karpenter-provider-aws/pull/9786) (#9786), which handles AMI drift by replacing the root volume.

## Motivation

`blockDeviceMappings` is part of the EC2NodeClass drift hash, so raising `iops` or `throughput` drifts and replaces every node on the EC2NodeClass. EBS can make that change on an attached volume with [`ModifyVolume`](https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_ModifyVolume.html), with no detach and no reboot. Karpenter never calls it.

gp3 limits went up to 80,000 IOPS and 2,000 MiB/s in September 2025 (#9823). Users throttled at the old 1,000 MiB/s cap can set the new values, but running nodes keep their old volumes until they're replaced. Raising the EC2NodeClass for one throttled node also raises cost on every other node. In EKS Auto Mode users can't call `ModifyVolume` on node volumes at all. They're EKS-managed, and the call is denied by a resource-based policy (aws/containers-roadmap#TODO).

### Use Cases

- **Emergency.** One node's data volume is at its throughput limit. On-call raises it on that node only, with an override.
- **Fleet change.** A platform team raises gp3 throughput on an EC2NodeClass, and every node gets it in place.
- **Back to baseline.** The override is removed, or the EC2NodeClass is lowered, after the incident.
- **Auto Mode.** The same, through `ephemeralStorage.iops` and `ephemeralStorage.throughput` on the NodeClass and through overrides.

### Non-Goals

- **Shrinking `volumeSize`.** EBS can't shrink a volume. Growing it through an override is in scope (see Growing size). Growing it through an EC2NodeClass change still drifts the node for now.
- **`volumeType`.** gp2 to gp3 is possible in place, but it changes pricing and limits, so it's left for later.
- **`kmsKeyID`, `encrypted`, `snapshotID` and `deleteOnTermination`.** These can't be changed in place.
- **CSI-attached volumes.** Those are covered by VolumeAttributesClass.

## Proposal

### Proposed Spec

**Opt-in for EC2NodeClass changes**, next to #9786's `amiDriftPolicy` and excluded from the drift hash the same way:

```yaml
apiVersion: karpenter.k8s.aws/v1
kind: EC2NodeClass
spec:
  blockDeviceDriftPolicy: ModifyVolume   # ReplaceInstance (default) | ModifyVolume
```

**Fields that can be updated without draining**:

| Field | Volume types |
|---|---|
| `blockDeviceMappings[].ebs.iops` | gp3, io1, io2 |
| `blockDeviceMappings[].ebs.throughput` | gp3 |
| `blockDeviceMappings[].ebs.volumeSize` | All EBS types, grow only, through the override only |

**Override schema** for `karpenter.sh/nodeclass-override`, keyed by device name so it can be partial:

```yaml
metadata:
  annotations:
    karpenter.sh/nodeclass-override: '{"blockDeviceMappings":{"/dev/xvdb":{"iops":16000,"throughput":1000}}}'
```

Only `iops` and `throughput` are accepted, on devices the EC2NodeClass maps. Overrides work whatever `blockDeviceDriftPolicy` is set to, because they're an explicit per-node action.

**Applied values**, written by the provider so drift checks don't rely on eventually consistent reads:

```yaml
metadata:
  annotations:
    karpenter.k8s.aws/applied-block-devices: '{"/dev/xvda":{"iops":3000,"throughput":125},"/dev/xvdb":{"iops":16000,"throughput":1000}}'
```

### How It Works

1. **Split the hash.** With `blockDeviceDriftPolicy: ModifyVolume`, `iops` and `throughput` move from the static hash to `karpenter.k8s.aws/ec2nodeclass-block-device-performance-hash`. The hash version is bumped, so existing NodeClaims are backfilled instead of drifting. With `ReplaceInstance` both hashes count as replacement drift, which is today's behavior.
2. **`GetDriftStatus`.**
   - It returns `Replace` or `Reboot` when another drift needs it. For example, AMI drift with `amiDriftPolicy: ReplaceRootVolume` gets `Reboot`.
   - It returns `Update` when the only difference is volume performance, compared against the EC2NodeClass with any override merged in.
   - It returns `NoOp` once `applied-block-devices` matches the target.
3. **Find the volumes.**
   - Get the volume IDs from `DescribeInstances` block device mappings, then call `DescribeVolumes` by ID.
   - Auto Mode instances and volumes are `Operator.HiddenByDefault`, so filtered queries return nothing. Only ID lookups work.
4. **`Update`.**
   - For each volume that differs, check `DescribeVolumesModifications`. If a modification toward the same target is already running, skip it. `ModifyVolume` has no client token, so this check is what makes retries safe.
   - Otherwise call `ModifyVolume` with the changed values.
   - `VolumeModificationRateExceeded` returns `NodeUpdateThrottledError`, with the time EBS gives in the message ("Wait until 2026-11-08T04:59:26.792Z ..."). Measured on gp3 in the prototype: EBS accepted 4 modifications to one volume and rejected the 5th, with a retry time exactly 24h after the first. So the limit is 4 per volume per rolling 24h, not one per 6h.
   - `IncorrectModificationState`, or a modification toward a different target still in `modifying` or `optimizing`, returns `NodeUpdateThrottledError` with a 1-minute retry. A volume takes one modification at a time. In the prototype, a throughput change spent 10 to 15 minutes in `optimizing` on a 30 GiB gp3 volume, and an urgent size growth had to wait behind it.
   - Validation errors and `UnauthorizedOperation` return `NodeUpdateFailedError`. Validation errors include IOPS above 500 per GiB, throughput above 0.25 MiB/s per IOPS, and values over the gp3 limits.
5. **Confirm.**
   - A modification goes through `modifying`, `optimizing` and `completed`. During `optimizing`, performance is only partway to the new values, so the update counts as done only at `completed`.
   - At that point the provider writes `applied-block-devices`, and `GetDriftStatus` returns `NoOp`.
   - Large volumes can take hours to reach `completed`.
6. **Validate overrides** on every drift pass. Unknown devices, unsupported volume types, fields other than `iops` and `throughput`, and values over EBS limits are rejected with a message. Core then sets `OverrideRejected`.

### Growing size

- The override may set `size`, which only grows. A smaller `size` in the override is rejected. An EC2NodeClass `volumeSize` below the current size, for example after growth, is ignored rather than treated as drift.
- Karpenter grows the block device with `ModifyVolume`. Growing the partition and filesystem has to happen on the node. In the prototype a privileged DaemonSet runs `growpart` and `xfs_growfs` when the block device is bigger than the partition. In EKS Auto Mode this belongs in the node agent.
- **The kubelet doesn't pick up the new capacity on its own.** In the prototype the filesystem grew from 20 to 30 GiB, but `status.capacity.ephemeral-storage` stayed at the launch value. Getting this right is KEP-3953 (`Node.Spec.ConfiguredCapacity`). Until then, extra space is usable but not schedulable, which still prevents disk-pressure evictions.

### Root volume replacement (#9786)

- `CreateReplaceRootVolumeTask` creates a new root volume from the AMI's snapshot. Whether it keeps root volume performance that was changed in place is TODO and needs a test.
- Either way, after the reboot the provider compares again. If the new root volume doesn't match the target, from the EC2NodeClass or an override, the next drift pass returns `Update`.
- A node with both AMI drift and performance drift gets `Reboot` first and `Update` second.

### EKS Auto Mode

The EKS-managed controller implements the same hooks for `ephemeralStorage.iops` and `ephemeralStorage.throughput` on the data volume (`/dev/xvdb`). The 4 GiB root volume is read-only and stays fixed. The controller can already reach managed volumes, and customers stay blocked by the resource-based deny. Overrides use the same annotation, with the key `ephemeralStorage` instead of `blockDeviceMappings`. A cluster admin can annotate NodeClaims in Auto Mode (verified with a server-side dry run).

### IAM

Scoped like `RebootInstances` and `CreateReplaceRootVolumeTask`:

```json
{
  "Sid": "AllowScopedModifyVolume",
  "Effect": "Allow",
  "Resource": "arn:${AWS::Partition}:ec2:${AWS::Region}:${AWS::AccountId}:volume/*",
  "Action": "ec2:ModifyVolume",
  "Condition": {
    "StringEquals": {"aws:ResourceTag/kubernetes.io/cluster/${ClusterName}": "owned"},
    "StringLike": {"aws:ResourceTag/karpenter.sh/nodepool": "*"}
  }
}
```

`ec2:DescribeVolumesModifications` joins `AllowRegionalReadActions`.

### Interaction with Existing Features

- **Instance EBS bandwidth.** Volume throughput above the instance's EBS bandwidth is accepted by EBS but can't be reached. The provider emits a warning event and doesn't block it.
- **Cost.** IOPS above 3,000 and throughput above 125 MiB/s are billed per volume. Events on each update give the old and new values.
- **Consolidation.** Overridden nodes are skipped (core RFC). EBS cost isn't in Karpenter's prices today.

### Observability

- Core's `Updating` condition, with messages like `2 volumes modifying, 1 optimizing`.
- An event per volume with the old and new values.
- `karpenter_ebs_volume_modifications_total{trigger, result}` and `karpenter_ebs_volume_modification_duration_seconds`.

### Edge Cases

- **A 5th change to one volume within 24h.** EBS rejects it with `VolumeModificationRateExceeded` and the time the next change is allowed. The NodeClaim stays `UpdateThrottled` until then. Back-to-back changes inside the limit work (override then revert, 16 seconds apart).
- **Instance-store-only nodes.** There's no EBS mapping, so overrides are rejected and drift is `NoOp`.
- **Volume gone out of band.** `DescribeVolumes` returns `NotFound`. Drift-driven updates fail and the node is replaced. Overrides fail and the node is kept.
- **A value invalid for that volume's size**, such as IOPS above 500 per GiB. Rejected before the call.

## Prototype Demo

A prototype of this design runs end to end on EKS (Kubernetes 1.35, ca-central-1), with modified Karpenter core (AndrewSirenko/karpenter `inplace-poc`) and provider-aws (AndrewSirenko/karpenter-provider-aws `inplace-poc`). The prototype covers per-node overrides only. Drift-driven `Update` isn't built yet.

Setup: one NodeClaim `inplace-lpcft` (c6a.large, `i-053cf8b312a3c128c`) from an EC2NodeClass whose root volume `/dev/xvda` is gp3, 20 GiB, 3000 IOPS, 125 MiB/s. It runs one pod.

Outputs are trimmed to the relevant columns. The full command log, scripts and mutation run are in [`test/inplace-poc/`](https://github.com/AndrewSirenko/karpenter-provider-aws/tree/inplace-poc/test/inplace-poc) on the provider-aws branch.

**Before**

```console
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 \
    --query 'Volumes[].{Iops:Iops,Throughput:Throughput,Size:Size,Type:VolumeType}' --output table
+------+-------+-------------+-------+
| Iops | Size  | Throughput  | Type  |
+------+-------+-------------+-------+
|  3000|  20   |  125        |  gp3  |
+------+-------+-------------+-------+
```

**Override one node**

```console
$ kubectl annotate nodeclaim inplace-lpcft \
    'karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500}}}'
nodeclaim.karpenter.sh/inplace-lpcft annotated

$ kubectl get nodeclaim inplace-lpcft -o jsonpath='{.status.conditions[?(@.type=="Updating")]}'
{"message":"provider is applying the update","reason":"UpdateIssued","status":"True","type":"Updating"}
```

About 20 seconds later the volume already reports the new values while EBS is still optimizing:

```console
$ aws ec2 describe-volumes-modifications --volume-ids vol-06834de2b44ed41f1 \
    --query 'VolumesModifications[].{State:ModificationState,FromIops:OriginalIops,ToIops:TargetIops,FromThroughput:OriginalThroughput,ToThroughput:TargetThroughput}'
State: optimizing   FromIops: 3000  ToIops: 6000   FromThroughput: 125  ToThroughput: 500
```

**After** (`completed` at 02:44:09Z, about 5 minutes after the annotation)

```console
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 \
    --query 'Volumes[].{Iops:Iops,Throughput:Throughput,Size:Size,Type:VolumeType}' --output table
+------+-------+-------------+-------+
| Iops | Size  | Throughput  | Type  |
+------+-------+-------------+-------+
|  6000|  20   |  500        |  gp3  |
+------+-------+-------------+-------+

$ kubectl get nodeclaim inplace-lpcft -o jsonpath='{.status.conditions[?(@.type=="Updating")]}'
{"message":"instance matches its target","reason":"UpdateSucceeded","status":"False","type":"Updating"}
```

The node was never drained or rebooted:

| Check | Before | After |
|---|---|---|
| Instance ID | i-053cf8b312a3c128c | i-053cf8b312a3c128c |
| Node boot ID | a678f864-8f81-441d-940f-d806bc49b978 | a678f864-8f81-441d-940f-d806bc49b978 |
| Pod UID | 401d48bd-da79-42a6-b9a2-b67739851cfc | 401d48bd-da79-42a6-b9a2-b67739851cfc |
| Pod restarts | 0 | 0 |
| `spec.unschedulable` | unset | unset |
| NodeClaims in the NodePool | 1 | 1 |

**Invalid override is rejected and changes nothing**

```console
$ kubectl annotate nodeclaim inplace-lpcft --overwrite \
    'karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvdz":{"iops":9000}}}'
$ kubectl get nodeclaim inplace-lpcft -o jsonpath='{.status.conditions[?(@.type=="Updating")].message}'
in-place update failed, device "/dev/xvdz" is not in EC2NodeClass "inplace" blockDeviceMappings
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query 'Volumes[0].[Iops,Throughput]' --output text
6000    500
```

**Remove the override, back to the NodeClass**

```console
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override-
# controller log: "modifying volume" fromIOPS=6000 toIOPS=3000 fromThroughput=500 toThroughput=125
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query 'Volumes[0].[Iops,Throughput]' --output text
3000    125
```

The revert went through 16 seconds after the first modification completed, and EBS didn't throttle it. It completed at 02:49:29Z with `UpdateSucceeded`, on the same instance and pod.

**Mutation check.** The same core with a provider built without the EC2 volume wiring rejects the override with `OverrideRejected` ("in-place update is not implemented by the cloud provider"), and the volume stays at 3000/125.

**Tests.** The core `nodeclaim.inplace` Ginkgo suite passes 7/7. Removing the step that records the applied override fails 2 of them.

## Alternatives Considered

### Let users call `ModifyVolume`

The self-managed workaround. Nothing reconciles it, and a later replacement silently drops it. Not possible in Auto Mode.

### Make it the default with no opt-in

Drainless is strictly better for these fields. But a default-on change would modify every running node's volumes on the next EC2NodeClass edit, and change cost along with them. The opt-in matches `amiDriftPolicy`. Revisit at beta.

## Backward Compatibility

- `blockDeviceDriftPolicy` defaults to `ReplaceInstance`, and overrides are off until core's `InPlaceOverrides` gate is enabled.
- Upgrading backfills the new hash without drifting nodes.
- Without the new IAM permissions, updates fail. Drift-driven updates fall back to replacement, and overrides report `UpdateFailed`.

## Graduation Criteria

- **Alpha.** Opt-in, with E2E tests for:
  - raising and lowering throughput through the EC2NodeClass;
  - an override applied and removed;
  - throttling;
  - a failed update falling back to replacement;
  - instance ID unchanged and no pod evicted.
- **Beta.** Production data on modification duration by volume size. Decide whether `ModifyVolume` becomes the default.

## Open Questions

1. Does `CreateReplaceRootVolumeTask` keep root volume IOPS and throughput that were changed in place? This needs a test.
2. Should the provider refuse throughput above the instance's EBS bandwidth, or only warn?
3. Should gp2 to gp3 `volumeType` changes join in a later RFC?
