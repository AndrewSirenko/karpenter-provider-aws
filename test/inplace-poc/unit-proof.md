# Core unit tests (poc-karpenter, 2026-11-06T02:39:49Z)

```
$ go test ./pkg/controllers/nodeclaim/inplace/... -count=1 -v
Ran 7 of 7 Specs in 7.121 seconds
SUCCESS! -- 7 Passed | 0 Failed | 0 Pending | 0 Skipped
--- PASS: TestAPIs (7.12s)
ok  	sigs.k8s.io/karpenter/pkg/controllers/nodeclaim/inplace	7.160s

# mutation: drop the applied-override marker (nc.Annotations[NodeClassOverrideAppliedAnnotationKey] = target)
FAIL! -- 5 Passed | 2 Failed | 0 Pending | 0 Skipped
  [FAIL] InPlace [It] issues the update and keeps polling until the provider reports done
  [FAIL] InPlace [It] reverts to the NodeClass when the override is removed
Ran 7 of 7 Specs in 7.222 seconds
```

# Throttle hot-loop fix (2026-11-07T06:10:14Z)

Bug found live: after EBS returned VolumeModificationRateExceeded, the controller re-called ModifyVolume about 3 times a second (condition message carried the request ID, so each status patch retriggered the reconcile). Fix: retry time kept in karpenter.sh/update-retry-after and honored before calling the provider, stable message, reconcile only on annotation/generation changes.

```
$ go test ./pkg/controllers/nodeclaim/inplace/... -count=1 -v   # mutation: retry-after gate removed
  [FAILED] Expected
  [FAIL] InPlace [It] doesn't call a throttled provider again before its retry time, however often it reconciles
  to equal
FAIL! -- 7 Passed | 1 Failed | 0 Pending | 0 Skipped
Ran 8 of 8 Specs in 7.061 seconds

$ go test ./pkg/controllers/nodeclaim/inplace/... -count=1 -v   # restored
Ran 8 of 8 Specs in 7.324 seconds
SUCCESS! -- 8 Passed | 0 Failed | 0 Pending | 0 Skipped
ok  	sigs.k8s.io/karpenter/pkg/controllers/nodeclaim/inplace	7.363s
```

## Live check of the throttle fix (2026-11-07T06:23:17Z)
```
$ kubectl get nodeclaim inplace-lpcft  (retry-after + Updating condition)
{"override":"{\"blockDeviceMappings\":{\"/dev/xvda\":{\"iops\":3000,\"size\":\"30Gi\",\"throughput\":500}}}","retryAfter":"2026-11-08T04:59:31Z","updating":{"status":"True","reason":"UpdateThrottled","message":"provider rate limit, retrying at 2026-11-08T04:59:31Z"}}
ModifyVolume calls in the Karpenter log: 1 at 06:21:44, 1 ninety seconds later
{"time":"2026-11-07T06:21:18.813Z","message":"in-place update throttled by the provider","retryAt":"2026-11-08T04:59:31.000Z"}
```

## CloudTrail: ModifyVolume on the POC account, 2026-11-07 04:50Z to 06:30Z (ReadOnly)
```
$ aws cloudtrail lookup-events --lookup-attributes AttributeKey=EventName,AttributeValue=ModifyVolume --start-time 2026-11-07T04:50:00Z --end-time 2026-11-07T06:30:00Z  (paged)
total 4935
   4931 Client.VolumeModificationRateExceeded
      4 OK
accepted:
2026-11-07T04:59:26+00:00
2026-11-07T05:04:39+00:00
2026-11-07T05:15:05+00:00
2026-11-07T05:30:21+00:00
rate-exceeded first/last:
2026-11-07T05:40:45+00:00
2026-11-07T06:21:18+00:00
```
