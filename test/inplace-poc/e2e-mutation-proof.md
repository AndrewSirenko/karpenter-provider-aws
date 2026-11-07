# Mutation e2e: provider built without WithVolumeAPI (2026-11-06T02:50:19Z)

Image 111122223333.dkr.ecr.ca-central-1.amazonaws.com/karpenter-poc:mutation-no-volumeapi@sha256:71554700da3b59099a02d80326ee6f40f4b45d3ee3a98620e8fb230b4644f4e8. NodeClaim inplace-lpcft, instance i-053cf8b312a3c128c, root volume vol-06834de2b44ed41f1.
```
$ kubectl -n kube-system get deploy karpenter -o jsonpath={.spec.template.spec.containers[0].image}{"\n"}
111122223333.dkr.ecr.ca-central-1.amazonaws.com/karpenter-poc:mutation-no-volumeapi@sha256:71554700da3b59099a02d80326ee6f40f4b45d3ee3a98620e8fb230b4644f4e8
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query Volumes[0].[Iops,Throughput] --output text
3000	125
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500}}} --overwrite
nodeclaim.karpenter.sh/inplace-lpcft annotated
$ kubectl get nodeclaim inplace-lpcft -o jsonpath={.status.conditions[?(@.type=="Updating")]}{"\n"}
{"lastTransitionTime":"2026-11-06T02:50:35Z","message":"in-place update is not implemented by the cloud provider","observedGeneration":1,"reason":"OverrideRejected","status":"False","type":"Updating"}
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query Volumes[0].[Iops,Throughput] --output text
3000	125
PASS: override rejected without the provider seam
PASS: volume unchanged (3000/125)
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override-
nodeclaim.karpenter.sh/inplace-lpcft annotated
$ kubectl -n kube-system get deploy karpenter -o jsonpath={.spec.template.spec.containers[0].image}{"\n"}
111122223333.dkr.ecr.ca-central-1.amazonaws.com/karpenter-poc:inplace-0207@sha256:d0b1622538efa8c57674f83ef1997b23167d0ecd74b5e8d5ac6d6a773f167019
```
