#!/usr/bin/env bash
# Mutation: same core, provider without the EC2 volume wiring. The override must be rejected and the volume must not change.
set -uo pipefail
export AWS_PROFILE=poc-dev AWS_DEFAULT_REGION=ca-central-1 AWS_REGION=ca-central-1
POC=/workplace/andsirey/agents/workstreams/ebs-update/poc
export KUBECONFIG=$POC/kubeconfig
OUT=$POC/e2e-mutation-proof.md; : > $OUT
log() { echo "$*" | tee -a $OUT; }
run() { log "\$ $*"; "$@" 2>&1 | tee -a $OUT; }
setimg() { local IMG=$1 REPO=${1%%:*} REST=${1#*:}; local TAG=${REST%%@*} DIGEST=${1#*@}
  kubectl -n kube-system set image deploy/karpenter controller=$REPO:$TAG@$DIGEST >/dev/null; kubectl -n kube-system rollout status deploy/karpenter --timeout=5m >/dev/null; }
NC=$(kubectl get nodeclaim -l karpenter.sh/nodepool=inplace -o jsonpath='{.items[0].metadata.name}')
IID=$(kubectl get nodeclaim $NC -o jsonpath='{.status.providerID}' | sed 's#.*/##')
ROOT=$(aws ec2 describe-instances --instance-ids $IID --query "Reservations[0].Instances[0].BlockDeviceMappings[?DeviceName=='/dev/xvda'].Ebs.VolumeId|[0]" --output text)
log "# Mutation e2e: provider built without WithVolumeAPI ($(date -u +%FT%TZ))"
log ""
log "Image $(cat $POC/image-mutation.txt). NodeClaim $NC, instance $IID, root volume $ROOT."
log '```'
setimg "$(cat $POC/image-mutation.txt)"
run kubectl -n kube-system get deploy karpenter -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
run aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[0].[Iops,Throughput]' --output text
run kubectl annotate nodeclaim $NC 'karpenter.sh/nodeclass-override={"blockDeviceMappings":{"/dev/xvda":{"iops":6000,"throughput":500}}}' --overwrite
sleep 90
run kubectl get nodeclaim $NC -o jsonpath='{.status.conditions[?(@.type=="Updating")]}{"\n"}'
run aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[0].[Iops,Throughput]' --output text
R=$(kubectl get nodeclaim $NC -o jsonpath='{.status.conditions[?(@.type=="Updating")].reason}')
V=$(aws ec2 describe-volumes --volume-ids $ROOT --query 'Volumes[0].[Iops,Throughput]' --output text | tr '\t' '/')
[ "$R" = OverrideRejected ] && log "PASS: override rejected without the provider seam" || log "FAIL: reason $R"
[ "$V" = 3000/125 ] && log "PASS: volume unchanged ($V)" || log "FAIL: volume $V"
run kubectl annotate nodeclaim $NC karpenter.sh/nodeclass-override-
setimg "$(cat $POC/image.txt)"
run kubectl -n kube-system get deploy karpenter -o jsonpath='{.spec.template.spec.containers[0].image}{"\n"}'
log '```'
