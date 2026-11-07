#!/usr/bin/env bash
# E2E: override one node's root volume IOPS/throughput in place, no drain, no reboot, no replacement.
set -uo pipefail
export AWS_PROFILE=poc-dev AWS_DEFAULT_REGION=ca-central-1 AWS_REGION=ca-central-1
POC=/workplace/andsirey/agents/workstreams/ebs-update/poc
export KUBECONFIG=$POC/kubeconfig
OUT=${OUT:-$POC/e2e-proof.md}
FAIL=0
log() { echo "$*" | tee -a $OUT; }
run() { log "\$ $*"; "$@" 2>&1 | tee -a $OUT; return ${PIPESTATUS[0]}; }
check() { if [ "$2" = "$3" ]; then log "PASS: $1 ($2)"; else log "FAIL: $1 (got '$2', want '$3')"; FAIL=1; fi; }
cond() { kubectl get nodeclaim $NC -o json | jq -r '.status.conditions[]? | select(.type=="Updating") | "\(.status)/\(.reason)/\(.message)"'; }
vol() { aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[0].[Iops,Throughput]' --output text | tr '\t' '/'; }
waitcond() { # $1 = regex on status/reason, $2 = timeout seconds
  local end=$(( $(date +%s) + $2 )); while [ $(date +%s) -lt $end ]; do c=$(cond); [[ "$c" =~ $1 ]] && { log "condition: $c"; return 0; }; sleep 15; done; log "timeout waiting for $1, last: $(cond)"; return 1; }

: > $OUT
log "# In-place NodeClaim override e2e ($(date -u +%FT%TZ))"
log ""
log "Cluster inplace-poc, ca-central-1, account 111122223333. Modified karpenter (poc-karpenter) + karpenter-provider-aws (poc-provider-aws), image $(cat $POC/image.txt)."
log '```'
run kubectl apply -f $POC/manifests.yaml
log "waiting for the inplace NodeClaim to initialize"
for i in $(seq 1 60); do
  NC=$(kubectl get nodeclaim -l karpenter.sh/nodepool=inplace -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  [ -n "$NC" ] && [ "$(kubectl get nodeclaim $NC -o jsonpath='{.status.conditions[?(@.type=="Initialized")].status}')" = True ] && break; sleep 10; done
run kubectl get nodeclaim -l karpenter.sh/nodepool=inplace -o wide
NODE=$(kubectl get nodeclaim $NC -o jsonpath='{.status.nodeName}')
IID=$(kubectl get nodeclaim $NC -o jsonpath='{.status.providerID}' | sed 's#.*/##')
ROOT=$(aws ec2 describe-instances --instance-ids $IID --query "Reservations[0].Instances[0].BlockDeviceMappings[?DeviceName=='/dev/xvda'].Ebs.VolumeId|[0]" --output text)
kubectl rollout status deploy/inplace-workload --timeout=5m >/dev/null
POD=$(kubectl get pod -l app=inplace-workload -o jsonpath='{.items[0].metadata.uid}')
BOOT=$(kubectl get node $NODE -o jsonpath='{.status.nodeInfo.bootID}')
log "NodeClaim=$NC Node=$NODE Instance=$IID RootVolume=$ROOT PodUID=$POD BootID=$BOOT"
log "## before"
run aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[].{VolumeId:VolumeId,Type:VolumeType,Size:Size,Iops:Iops,Throughput:Throughput}' --output table
check "root volume before" "$(vol)" "3000/125"

log "## 1. override one node: /dev/xvda to 6000 IOPS, 500 MiB/s"
run kubectl annotate nodeclaim $NC 'karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500}}}' --overwrite
waitcond '^(True/UpdateIssued|False/UpdateSucceeded)' 300
for i in $(seq 1 6); do log "$(date -u +%T) volume=$(vol) condition=$(cond)"; [[ "$(cond)" =~ ^False/UpdateSucceeded ]] && break; sleep 20; done
run aws ec2 describe-volumes-modifications --volume-ids $ROOT --query 'VolumesModifications[].{State:ModificationState,Progress:Progress,FromIops:OriginalIops,ToIops:TargetIops,FromThroughput:OriginalThroughput,ToThroughput:TargetThroughput,Start:StartTime}' --output table
waitcond '^False/UpdateSucceeded' 3600
log "## after"
run aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[].{VolumeId:VolumeId,Type:VolumeType,Size:Size,Iops:Iops,Throughput:Throughput}' --output table
run aws ec2 describe-volumes-modifications --volume-ids $ROOT --query 'VolumesModifications[].{State:ModificationState,Progress:Progress,ToIops:TargetIops,ToThroughput:TargetThroughput,End:EndTime}' --output table
run kubectl get nodeclaim $NC -o jsonpath='{.metadata.annotations}{"\n"}{.status.conditions[?(@.type=="Updating")]}{"\n"}'
check "root volume after" "$(vol)" "6000/500"
check "Updating condition" "$(cond | cut -d/ -f1-2)" "False/UpdateSucceeded"
check "same NodeClaim instance" "$(kubectl get nodeclaim $NC -o jsonpath='{.status.providerID}' | sed 's#.*/##')" "$IID"
check "node not rebooted (bootID)" "$(kubectl get node $NODE -o jsonpath='{.status.nodeInfo.bootID}')" "$BOOT"
check "node not cordoned" "$(kubectl get node $NODE -o jsonpath='{.spec.unschedulable}')" ""
check "pod not evicted (UID)" "$(kubectl get pod -l app=inplace-workload -o jsonpath='{.items[0].metadata.uid}')" "$POD"
check "pod restarts" "$(kubectl get pod -l app=inplace-workload -o jsonpath='{.items[0].status.containerStatuses[0].restartCount}')" "0"
check "only one inplace NodeClaim" "$(kubectl get nodeclaim -l karpenter.sh/nodepool=inplace -o name | wc -l | tr -d ' ')" "1"

log "## 2. invalid override is rejected and changes nothing"
run kubectl annotate nodeclaim $NC 'karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvdz":{"iops":9000}}}' --overwrite
waitcond '^False/OverrideRejected' 300
check "rejected override leaves volume" "$(vol)" "6000/500"

log "## 3. removing the override reverts to the EC2NodeClass, throttled by the EBS 6h cooldown"
run kubectl annotate nodeclaim $NC karpenter.sh/nodeclass-override-
waitcond '^True/(UpdateThrottled|UpdateIssued)' 300
run kubectl get nodeclaim $NC -o jsonpath='{.status.conditions[?(@.type=="Updating")]}{"\n"}'
log "## controller logs"
kubectl -n kube-system logs deploy/karpenter --since=2h 2>/dev/null | rg 'nodeclaim.inplace|in-place|modifying volume' | tail -25 | tee -a $OUT
log '```'
[ $FAIL = 0 ] && log "RESULT: PASS" || log "RESULT: FAIL"
exit $FAIL
