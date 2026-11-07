#!/usr/bin/env bash
set -euo pipefail
export AWS_PROFILE=poc-dev AWS_DEFAULT_REGION=ca-central-1 AWS_REGION=ca-central-1
POC=/workplace/andsirey/agents/workstreams/ebs-update/poc
export KUBECONFIG=$POC/kubeconfig
CLUSTER_NAME=inplace-poc
IMG=$(cat $POC/image.txt)
REPO=${IMG%%:*}; REST=${IMG#*:}; TAG=${REST%%@*}; DIGEST=${IMG#*@}
aws iam put-role-policy --role-name ${CLUSTER_NAME}-karpenter --policy-name InPlaceVolumePOC --policy-document '{
  "Version":"2012-10-17",
  "Statement":[{"Effect":"Allow","Action":["ec2:ModifyVolume","ec2:DescribeVolumes","ec2:DescribeVolumesModifications"],"Resource":"*"}]}'
helm upgrade --install karpenter /workplace/andsirey/agents/workstreams/ebs-update/poc-provider-aws/charts/karpenter -n kube-system \
  --set settings.clusterName=$CLUSTER_NAME --set settings.interruptionQueue=$CLUSTER_NAME \
  --set controller.image.repository=$REPO --set controller.image.tag=$TAG --set controller.image.digest=$DIGEST \
  --set replicas=1 --set controller.resources.requests.cpu=1 --set controller.resources.requests.memory=1Gi \
  --set controller.resources.limits.cpu=1 --set controller.resources.limits.memory=1Gi --wait --timeout 10m
kubectl -n kube-system rollout restart deploy/karpenter
kubectl -n kube-system rollout status deploy/karpenter --timeout=5m
kubectl -n kube-system get pods -l app.kubernetes.io/name=karpenter -o wide
