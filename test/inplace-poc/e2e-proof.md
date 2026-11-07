# In-place NodeClaim override e2e (2026-11-06T02:37:56Z)

Cluster inplace-poc, ca-central-1, account 111122223333. Modified karpenter (poc-karpenter) + karpenter-provider-aws (poc-provider-aws), image 111122223333.dkr.ecr.ca-central-1.amazonaws.com/karpenter-poc:inplace-0207@sha256:d0b1622538efa8c57674f83ef1997b23167d0ecd74b5e8d5ac6d6a773f167019.
```
$ kubectl apply -f /workplace/andsirey/agents/workstreams/ebs-update/poc/manifests.yaml
ec2nodeclass.karpenter.k8s.aws/inplace created
nodepool.karpenter.sh/inplace created
deployment.apps/inplace-workload created
waiting for the inplace NodeClaim to initialize
$ kubectl get nodeclaim -l karpenter.sh/nodepool=inplace -o wide
NAME            TYPE        CAPACITY    ZONE            NODE                                             READY   AGE   IMAGEID                 ID                                         NODEPOOL   NODECLASS   DRIFTED
inplace-lpcft   c6a.large   on-demand   ca-central-1b   ip-192-168-75-88.ca-central-1.compute.internal   True    42s   ami-04addfb726de472c0   aws:///ca-central-1b/i-053cf8b312a3c128c   inplace    inplace     
NodeClaim=inplace-lpcft Node=ip-192-168-75-88.ca-central-1.compute.internal Instance=i-053cf8b312a3c128c RootVolume=vol-06834de2b44ed41f1 PodUID=401d48bd-da79-42a6-b9a2-b67739851cfc BootID=a678f864-8f81-441d-940f-d806bc49b978
## before
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query Volumes[].{VolumeId:VolumeId,Type:VolumeType,Size:Size,Iops:Iops,Throughput:Throughput} --output table
-----------------------------------------------------------------
|                        DescribeVolumes                        |
+------+-------+-------------+-------+--------------------------+
| Iops | Size  | Throughput  | Type  |        VolumeId          |
+------+-------+-------------+-------+--------------------------+
|  3000|  20   |  125        |  gp3  |  vol-06834de2b44ed41f1   |
+------+-------+-------------+-------+--------------------------+
PASS: root volume before (3000/125)
## 1. override one node: /dev/xvda to 6000 IOPS, 500 MiB/s
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500}}} --overwrite
nodeclaim.karpenter.sh/inplace-lpcft annotated
condition: True/UpdateIssued/provider is applying the update
02:39:13 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
02:39:35 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
02:39:57 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
02:40:19 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
02:40:41 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
02:41:03 volume=6000/500 condition=True/UpdateIssued/provider is applying the update
$ aws ec2 describe-volumes-modifications --volume-ids vol-06834de2b44ed41f1 --query VolumesModifications[].{State:ModificationState,Progress:Progress,FromIops:OriginalIops,ToIops:TargetIops,FromThroughput:OriginalThroughput,ToThroughput:TargetThroughput,Start:StartTime} --output table
-------------------------------------------------
|         DescribeVolumesModifications          |
+-----------------+-----------------------------+
|  FromIops       |  3000                       |
|  FromThroughput |  125                        |
|  Progress       |  0                          |
|  Start          |  2026-11-06T02:38:58+00:00  |
|  State          |  optimizing                 |
|  ToIops         |  6000                       |
|  ToThroughput   |  500                        |
+-----------------+-----------------------------+
condition: False/UpdateSucceeded/instance matches its target
## after
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1 --query Volumes[].{VolumeId:VolumeId,Type:VolumeType,Size:Size,Iops:Iops,Throughput:Throughput} --output table
-----------------------------------------------------------------
|                        DescribeVolumes                        |
+------+-------+-------------+-------+--------------------------+
| Iops | Size  | Throughput  | Type  |        VolumeId          |
+------+-------+-------------+-------+--------------------------+
|  6000|  20   |  500        |  gp3  |  vol-06834de2b44ed41f1   |
+------+-------+-------------+-------+--------------------------+
$ aws ec2 describe-volumes-modifications --volume-ids vol-06834de2b44ed41f1 --query VolumesModifications[].{State:ModificationState,Progress:Progress,ToIops:TargetIops,ToThroughput:TargetThroughput,End:EndTime} --output table
-----------------------------------------------
|        DescribeVolumesModifications         |
+---------------+-----------------------------+
|  End          |  2026-11-06T02:44:09+00:00  |
|  Progress     |  100                        |
|  State        |  completed                  |
|  ToIops       |  6000                       |
|  ToThroughput |  500                        |
+---------------+-----------------------------+
$ kubectl get nodeclaim inplace-lpcft -o jsonpath={.metadata.annotations}{"\n"}{.status.conditions[?(@.type=="Updating")]}{"\n"}
{"compatibility.karpenter.k8s.aws/cluster-name-tagged":"true","karpenter.k8s.aws/ec2nodeclass-hash":"3771914003469259622","karpenter.k8s.aws/ec2nodeclass-hash-version":"v6","karpenter.k8s.aws/instance-profile-name":"inplace-poc_10307401474569856070","karpenter.k8s.aws/tagged":"true","karpenter.sh/nodeclaim-min-values-relaxed":"false","karpenter.sh/nodeclass-override":"{\"blockDeviceMappings\":{\"/dev/xvda\":{\"iops\":6000,\"throughput\":500}}}","karpenter.sh/nodeclass-override-applied":"0d07af0a9a283ea2","karpenter.sh/nodepool-hash":"10596855384235334465","karpenter.sh/nodepool-hash-version":"v3","karpenter.sh/update-operation-id":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2"}
{"lastTransitionTime":"2026-11-06T02:44:10Z","message":"instance matches its target","observedGeneration":1,"reason":"UpdateSucceeded","status":"False","type":"Updating"}
PASS: root volume after (6000/500)
PASS: Updating condition (False/UpdateSucceeded)
PASS: same NodeClaim instance (i-053cf8b312a3c128c)
PASS: node not rebooted (bootID) (a678f864-8f81-441d-940f-d806bc49b978)
PASS: node not cordoned ()
PASS: pod not evicted (UID) (401d48bd-da79-42a6-b9a2-b67739851cfc)
PASS: pod restarts (0)
PASS: only one inplace NodeClaim (1)
## 2. invalid override is rejected and changes nothing
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvdz":{"iops":9000}}} --overwrite
nodeclaim.karpenter.sh/inplace-lpcft annotated
condition: False/OverrideRejected/in-place update failed, device "/dev/xvdz" is not in EC2NodeClass "inplace" blockDeviceMappings
PASS: rejected override leaves volume (6000/500)
## 3. removing the override reverts to the EC2NodeClass (expected the EBS 6h cooldown to throttle this; it did not)
$ kubectl annotate nodeclaim inplace-lpcft karpenter.sh/nodeclass-override-
nodeclaim.karpenter.sh/inplace-lpcft annotated
condition: True/UpdateIssued/provider is applying the update
$ kubectl get nodeclaim inplace-lpcft -o jsonpath={.status.conditions[?(@.type=="Updating")]}{"\n"}
{"lastTransitionTime":"2026-11-06T02:44:24Z","message":"provider is applying the update","observedGeneration":1,"reason":"UpdateIssued","status":"True","type":"Updating"}
## controller logs
{"level":"INFO","time":"2026-11-06T02:40:07.516Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"f2bf3fe3-c428-4702-9569-712c69459ce7","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:40:22.687Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"dcff0a89-14ad-4313-a2cd-91af6c0f835b","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:40:37.855Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"94fd24f2-fcda-455e-8664-af526974b5f6","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:40:53.029Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"31d5fe74-dadc-49f0-a7d7-d9c0e9b48aa2","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:41:08.208Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"5dae5057-20f5-46a9-88e6-399392692840","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:41:23.391Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"f0303e34-64d9-4aea-93c7-fe5e1cbaf39f","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:41:38.574Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"830582f6-7f26-4a14-a912-37235ff22d83","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:41:53.797Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"3fa6d4d2-8881-43bb-9f68-025d3f385b27","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:42:08.966Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"ba0815f6-a292-43f1-859a-bad2a992a212","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:42:24.142Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"02b1715f-8d12-44ce-848d-17f5add76f35","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:42:39.325Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"9ba3b1d2-c679-4475-94d4-cf37ca2586af","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:42:54.508Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"6ec7e5c9-c8fc-4787-997b-94e150ad1f7d","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:43:09.693Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"6c4b8d42-fb5f-4025-adb4-6f5c05d04826","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:43:24.869Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"a6ba77fa-6dea-4fc0-b1d4-6cdc08adf880","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:43:40.059Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"04a7d322-7352-4ca2-83ae-ee8719dfb911","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:43:55.246Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"3ac6c3ad-604d-4cc4-b8ad-1bda805763d5","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
{"level":"INFO","time":"2026-11-06T02:44:10.497Z","logger":"controller","message":"in-place update succeeded","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"20ab4702-0e23-4257-8944-6be2f89d9245","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-0d07af0a9a283ea2"}
{"level":"INFO","time":"2026-11-06T02:44:22.821Z","logger":"controller","message":"in-place update requested","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"3b6fb67a-71a8-4c5d-b001-24dfd7013d88","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-aa807147ba86b32f","override":"{\"blockDeviceMappings\":{\"/dev/xvdz\":{\"iops\":9000}}}"}
{"level":"ERROR","time":"2026-11-06T02:44:22.821Z","logger":"controller","message":"in-place update failed","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"3b6fb67a-71a8-4c5d-b001-24dfd7013d88","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-aa807147ba86b32f","error":"in-place update failed, device \"/dev/xvdz\" is not in EC2NodeClass \"inplace\" blockDeviceMappings"}
{"level":"INFO","time":"2026-11-06T02:44:24.974Z","logger":"controller","message":"in-place update requested","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"93c162bc-d184-451f-8e9c-3a01782da7c1","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","override":""}
{"level":"INFO","time":"2026-11-06T02:44:25.147Z","logger":"controller","message":"modifying volume","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"93c162bc-d184-451f-8e9c-3a01782da7c1","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","device":"/dev/xvda","volume":"vol-06834de2b44ed41f1","fromIOPS":6000,"toIOPS":3000,"fromThroughput":500,"toThroughput":125}
{"level":"INFO","time":"2026-11-06T02:44:25.289Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"93c162bc-d184-451f-8e9c-3a01782da7c1","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","volumes":"/dev/xvda vol-06834de2b44ed41f1 modify requested"}
{"level":"INFO","time":"2026-11-06T02:44:25.490Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"848ebfba-5668-40a4-90c8-50a3f27937f1","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","volumes":"/dev/xvda vol-06834de2b44ed41f1 modifying 0%"}
{"level":"INFO","time":"2026-11-06T02:44:25.670Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"33280f78-35ed-4f43-9579-ba8f6ce553c8","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","volumes":"/dev/xvda vol-06834de2b44ed41f1 modifying 0%"}
{"level":"INFO","time":"2026-11-06T02:44:40.895Z","logger":"controller","message":"in-place volume update in progress","commit":"0abee51-dirty","controller":"nodeclaim.inplace","controllerGroup":"karpenter.sh","controllerKind":"NodeClaim","NodeClaim":{"name":"inplace-lpcft"},"namespace":"","name":"inplace-lpcft","reconcileID":"afb5981c-f6e0-4ded-b615-da5e0373086c","Node":{"name":"ip-192-168-75-88.ca-central-1.compute.internal"},"id":"i-053cf8b312a3c128c","operationID":"636605cd-4c19-480c-9933-37e0f8a0f856-e3b0c44298fc1c14","volumes":"/dev/xvda vol-06834de2b44ed41f1 optimizing 0%"}
```
RESULT: PASS

## 4. revert follow-up (2026-11-06T02:45:02Z)
```
$ aws ec2 describe-volumes-modifications --volume-ids vol-06834de2b44ed41f1
-------------------------------------------------
|         DescribeVolumesModifications          |
+-----------------+-----------------------------+
|  End            |  None                       |
|  FromIops       |  6000                       |
|  FromThroughput |  500                        |
|  Progress       |  0                          |
|  Start          |  2026-11-06T02:44:25+00:00  |
|  State          |  optimizing                 |
|  ToIops         |  3000                       |
|  ToThroughput   |  125                        |
+-----------------+-----------------------------+
$ aws ec2 describe-volumes --volume-ids vol-06834de2b44ed41f1
--------------------------------
|        DescribeVolumes       |
+------+--------+--------------+
| Iops | Size   | Throughput   |
+------+--------+--------------+
|  3000|  20    |  125         |
+------+--------+--------------+
$ kubectl get nodeclaim inplace-lpcft Updating condition + override annotations
{"override":null,"applied":"0d07af0a9a283ea2","updating":{"lastTransitionTime":"2026-11-06T02:44:24Z","message":"provider is applying the update","observedGeneration":1,"reason":"UpdateIssued","status":"True","type":"Updating"}}
$ kubectl get nodeclaim -l karpenter.sh/nodepool=inplace; pod uid; node bootID
inplace-lpcft   c6a.large   on-demand   ca-central-1b   ip-192-168-75-88.ca-central-1.compute.internal   True   6m57s
401d48bd-da79-42a6-b9a2-b67739851cfc restarts=0
a678f864-8f81-441d-940f-d806bc49b978
```

## 5. revert completed (2026-11-06T02:49:56Z)
```
-----------------------------------------------
|        DescribeVolumesModifications         |
+---------------+-----------------------------+
|  End          |  2026-11-06T02:49:29+00:00  |
|  Start        |  2026-11-06T02:44:25+00:00  |
|  State        |  completed                  |
|  ToIops       |  3000                       |
|  ToThroughput |  125                        |
+---------------+-----------------------------+
------------------------
|    DescribeVolumes   |
+-------+--------------+
| Iops  | Throughput   |
+-------+--------------+
|  3000 |  125         |
+-------+--------------+
{"override":null,"applied":null,"updating":{"status":"False","reason":"UpdateSucceeded","message":"instance matches its target"}}
pod 401d48bd-da79-42a6-b9a2-b67739851cfc restarts=0
bootID a678f864-8f81-441d-940f-d806bc49b978 unschedulable=
```
