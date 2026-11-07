#!/usr/bin/env bash
set -euo pipefail
export AWS_PROFILE=poc-dev AWS_DEFAULT_REGION=ca-central-1 AWS_REGION=ca-central-1
CLUSTER_NAME=inplace-poc K8S_VERSION=1.35 NS=kube-system
P=/workplace/andsirey/agents/workstreams/ebs-update/poc-provider-aws
POC=/workplace/andsirey/agents/workstreams/ebs-update/poc
ACCT=$(aws sts get-caller-identity --query Account --output text)
aws cloudformation deploy --stack-name "Karpenter-${CLUSTER_NAME}" \
  --template-file $P/website/content/en/preview/getting-started/getting-started-with-karpenter/cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM --parameter-overrides "ClusterName=${CLUSTER_NAME}"
cat > $POC/cluster.yaml <<YAML
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: ${CLUSTER_NAME}
  region: ca-central-1
  version: "${K8S_VERSION}"
  tags:
    karpenter.sh/discovery: ${CLUSTER_NAME}
    workstream: ebs-update
iam:
  withOIDC: true
  podIdentityAssociations:
  - namespace: "${NS}"
    serviceAccountName: karpenter
    roleName: ${CLUSTER_NAME}-karpenter
    permissionPolicyARNs:
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerNodeLifecyclePolicy-${CLUSTER_NAME}
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerIAMIntegrationPolicy-${CLUSTER_NAME}
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerEKSIntegrationPolicy-${CLUSTER_NAME}
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerInterruptionPolicy-${CLUSTER_NAME}
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerResourceDiscoveryPolicy-${CLUSTER_NAME}
    - arn:aws:iam::${ACCT}:policy/KarpenterControllerZonalShiftPolicy-${CLUSTER_NAME}
iamIdentityMappings:
- arn: "arn:aws:iam::${ACCT}:role/KarpenterNodeRole-${CLUSTER_NAME}"
  username: system:node:{{EC2PrivateDNSName}}
  groups: [system:bootstrappers, system:nodes]
managedNodeGroups:
- instanceType: m5.large
  amiFamily: AmazonLinux2023
  name: ${CLUSTER_NAME}-ng
  desiredCapacity: 2
  minSize: 1
  maxSize: 3
addons:
- name: eks-pod-identity-agent
YAML
KUBECONFIG=$POC/kubeconfig eksctl create cluster -f $POC/cluster.yaml
