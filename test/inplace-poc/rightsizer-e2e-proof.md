# Volume right-sizer e2e (2026-11-07T06:29:55Z)

Cluster inplace-poc (ca-central-1). Karpenter 111122223333.dkr.ecr.ca-central-1.amazonaws.com/karpenter-poc:inplace-size-0454. Right-sizer running out of cluster in apply mode (hack/run-local.sh: interval 20s, window 2m, grow at 70% by 1.5x, raise at 80% x2, lower after 10m under 30%, min 2m between changes, 4 changes per volume per 24h with the last kept for growth, caps 60GiB/8000/500).
NodeClaim inplace-mwh6b, node ip-192-168-161-54.ca-central-1.compute.internal, instance i-036c5d8879e7064bf, root volume vol-06fcd1678bd609f7d.
```
06:29:55 volume=20/3000/125 lastmod= fs=[3GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=
## 1. a write-heavy workload: one pod fills the disk (dd 12GiB), one keeps rewriting 2GiB with O_DIRECT
pod/disk-filler created
pod/io-load created
06:30:33 volume=20/3000/250 lastmod=optimizing/20/3000/250 fs=[8GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
reached: throughput raised above 125 MiB/s (20/3000/250)
06:31:26 volume=20/3000/250 lastmod=optimizing/20/3000/250 fs=[15GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:32:37 volume=20/3000/250 lastmod=optimizing/20/3000/250 fs=[17GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:33:52 volume=20/3000/250 lastmod=optimizing/20/3000/250 fs=[16GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:35:06 volume=20/3000/250 lastmod=optimizing/20/3000/250 fs=[15GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:35:57 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[16GiB used of 19GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
reached: volume grew past 20GiB (30/3000/500)
06:36:07 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[17GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
reached: filesystem grew on the node (17GiB used of 29GiB)
$ kubectl -n kube-system logs -l app=volume-rightsizer-fsgrow --tail=4
         =                       sectsz=4096  sunit=4 blks, lazy-count=1
realtime =none                   extsz=4096   blocks=0, rtextents=0
data blocks changed from 5239803 to 7861243
2026-11-07T06:35:58Z grown: /dev/nvme0n1p1   30G   18G   13G  58% /
kubelet ephemeral-storage capacity after the filesystem grew: 20893676Ki
## 2. load continues; the next raise uses the 3rd of EBS's 4 changes per 24h
06:36:21 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[17GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
reached: throughput at the 500 MiB/s cap (30/3000/500)
## 3. load stops; lowering would be the 4th change, which the right-sizer keeps for growth
$ kubectl delete pod io-load --wait=true
pod "io-load" deleted from default namespace
06:37:44 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:38:58 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:40:11 volume=30/3000/500 lastmod=optimizing/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:41:22 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:42:33 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:43:43 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:44:55 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:46:07 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:47:19 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:48:29 volume=30/3000/500 lastmod=completed/30/3000/500 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateSucceeded
06:49:38 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:50:47 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:51:58 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:53:12 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:54:25 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:55:37 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:56:49 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:58:02 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
06:59:14 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
07:00:28 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
07:01:42 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
07:02:09 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=20893676Ki Updating=UpdateIssued
TIMEOUT: lowering held by the daily change budget (last )
## 4. does the kubelet pick up the grown filesystem on restart?
before: 20893676Ki
$ kubectl -n kube-system exec pod/volume-rightsizer-fsgrow-9wxm7 -- nsenter -t 1 -m -p -- systemctl restart kubelet
07:02:26 volume=30/3000/125 lastmod=optimizing/30/3000/125 fs=[15GiB used of 29GiB] node.capacity.ephemeral-storage=31379436Ki Updating=UpdateIssued
reached: kubelet reports the new ephemeral-storage capacity after restart (31379436Ki)
$ aws ec2 describe-volumes-modifications --volume-ids vol-06fcd1678bd609f7d --query VolumesModifications[].{Start:StartTime,End:EndTime,State:ModificationState,ToSize:TargetSize,ToIops:TargetIops,ToMiBps:TargetThroughput} --output table
------------------------------------------
|      DescribeVolumesModifications      |
+----------+-----------------------------+
|  End     |  None                       |
|  Start   |  2026-11-07T06:49:18+00:00  |
|  State   |  optimizing                 |
|  ToIops  |  3000                       |
|  ToMiBps |  125                        |
|  ToSize  |  30                         |
+----------+-----------------------------+
## checks
PASS: same instance (i-036c5d8879e7064bf)
PASS: node not rebooted (bootID) (ee267d24-311d-4cf7-a8ff-fc3943e66f62)
PASS: node never cordoned ()
PASS: filling pod not evicted (UID) (71e72a3f-3e76-4c1b-a2a0-3c60437335b0)
PASS: no DiskPressure (False)
PASS: pods on the node survived the kubelet restart (Running)
PASS: size never shrank (grown)
$ kubectl get events --field-selector involvedObject.name=inplace-mwh6b -o custom-columns=TIME:.lastTimestamp,REASON:.reason,MESSAGE:.message
TIME                   REASON              MESSAGE
2026-11-07T06:27:31Z   Launched            Status condition transitioned, Type: Launched, Status: Unknown -> True, Reason: Launched
2026-11-07T06:27:31Z   DisruptionBlocked   Nodeclaim does not have an associated node
2026-11-07T06:27:46Z   Registered          Status condition transitioned, Type: Registered, Status: Unknown -> True, Reason: Registered
2026-11-07T06:27:58Z   Initialized         Status condition transitioned, Type: Initialized, Status: Unknown -> True, Reason: Initialized
2026-11-07T06:27:58Z   Ready               Status condition transitioned, Type: Ready, Status: Unknown -> True, Reason: Ready
2026-11-07T06:29:02Z   Unconsolidatable    Can't replace with a cheaper node
2026-11-07T06:30:27Z   VolumeRightsized    /dev/xvda: 20GiB/3000 IOPS/125 MiB/s -> 20GiB/3000 IOPS/250 MiB/s: peak 104 MiB/s >= 80% of 125, raise to 250
2026-11-07T06:41:07Z   Updating            Status condition transitioned, Type: Updating, Status: True -> False, Reason: UpdateSucceeded, Message: instance matches its target
2026-11-07T06:35:47Z   VolumeRightsized    /dev/xvda: 20GiB/3000 IOPS/250 MiB/s -> 30GiB/3000 IOPS/500 MiB/s: filesystem 88% full >= 70%, grow 20GiB to 30GiB; peak 328 MiB/s >= 80% of 250, raise to 500
2026-11-07T06:49:17Z   Updating            Status condition transitioned, Type: Updating, Status: False -> True, Reason: UpdateRequested, Message: applying override
2026-11-07T06:49:17Z   VolumeRightsized    /dev/xvda: 30GiB/3000 IOPS/500 MiB/s -> 30GiB/3000 IOPS/125 MiB/s: peak 0 MiB/s < 30% of 500 for 10m0s, lower to 125
$ kubectl get nodeclaim inplace-mwh6b -o jsonpath={.metadata.annotations.rightsizer\.eks\.amazonaws\.com/changes}{"\n"}
2026-11-07T06:30:26Z,2026-11-07T06:35:47Z,2026-11-07T06:49:17Z
```
right-sizer log (applied recommendations):
```
{"ts":"2026-11-07T06:30:27Z","msg":"applied volume recommendation","outcome":null,"from":null,"to":null,"reasons":["peak 104 MiB/s >= 80% of 125, raise to 250"]}
{"ts":"2026-11-07T06:30:27Z","msg":"volume recommendation","outcome":"applied","from":{"SizeGiB":20,"IOPS":3000,"MiBps":125},"to":{"SizeGiB":20,"IOPS":3000,"MiBps":250},"reasons":["peak 104 MiB/s >= 80% of 125, raise to 250"]}
{"ts":"2026-11-07T06:35:47Z","msg":"applied volume recommendation","outcome":null,"from":null,"to":null,"reasons":["filesystem 88% full >= 70%, grow 20GiB to 30GiB","peak 328 MiB/s >= 80% of 250, raise to 500"]}
{"ts":"2026-11-07T06:35:47Z","msg":"volume recommendation","outcome":"applied","from":{"SizeGiB":20,"IOPS":3000,"MiBps":250},"to":{"SizeGiB":30,"IOPS":3000,"MiBps":500},"reasons":["filesystem 88% full >= 70%, grow 20GiB to 30GiB","peak 328 MiB/s >= 80% of 250, raise to 500"]}
{"ts":"2026-11-07T06:49:17Z","msg":"applied volume recommendation","outcome":null,"from":null,"to":null,"reasons":["peak 0 MiB/s < 30% of 500 for 10m0s, lower to 125"]}
{"ts":"2026-11-07T06:49:17Z","msg":"volume recommendation","outcome":"applied","from":{"SizeGiB":30,"IOPS":3000,"MiBps":500},"to":{"SizeGiB":30,"IOPS":3000,"MiBps":125},"reasons":["peak 0 MiB/s < 30% of 500 for 10m0s, lower to 125"]}
```
RESULT: FAIL

## 3b. correction and budget demo (2026-11-07T07:04:54Z)

Step 3's expectation was wrong: the recommender batched growth and the raise to 500 into one change (06:35:47), so lowering to 125 at 06:49 was the 3rd change and allowed. With 3 used, a performance-only change is held so the 4th stays available for growth:
```
$ kubectl get nodeclaim inplace-mwh6b -o jsonpath='{.metadata.annotations.rightsizer\.eks\.amazonaws\.com/changes}'
2026-11-07T06:30:26Z,2026-11-07T06:35:47Z,2026-11-07T06:49:17Z
$ kubectl apply io-load   (sustained O_DIRECT writes again)
$ kubectl get events --field-selector involvedObject.name=inplace-mwh6b,reason=VolumeRecommendationHeld
TIME                   MESSAGE
2026-11-07T07:05:42Z   3 of 4 changes used in 24h, the last is kept for growing size: /dev/xvda: 30GiB/3000 IOPS/125 MiB/s -> 30GiB/3000 IOPS/250 MiB/s: peak 123 MiB/s >= 80% of 125, raise to 250
$ aws ec2 describe-volumes --volume-ids vol-06fcd1678bd609f7d
30	3000	125
```
PASS: performance-only raise held by the daily budget
PASS: volume unchanged by the held recommendation (125 MiB/s)
RESULT (after correction): PASS
