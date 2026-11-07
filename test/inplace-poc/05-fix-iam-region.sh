#!/usr/bin/env bash
set -euxo pipefail
export AWS_PROFILE=poc-dev
R=inplace-poc-karpenter; ACCT=111122223333
POLS=$(aws iam list-attached-role-policies --role-name $R --query 'AttachedPolicies[?starts_with(PolicyName,`KarpenterController`)].PolicyArn' --output text)
for p in $POLS; do aws iam detach-role-policy --role-name $R --policy-arn $p; done
aws cloudformation delete-stack --region us-west-2 --stack-name Karpenter-inplace-poc
aws cloudformation wait stack-delete-complete --region us-west-2 --stack-name Karpenter-inplace-poc
aws ecr delete-repository --region us-west-2 --repository-name karpenter-poc --force >/dev/null || true
aws cloudformation deploy --region ca-central-1 --stack-name Karpenter-inplace-poc \
  --template-file /workplace/andsirey/agents/workstreams/ebs-update/poc-provider-aws/website/content/en/preview/getting-started/getting-started-with-karpenter/cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM --parameter-overrides ClusterName=inplace-poc
for p in $POLS; do aws iam attach-role-policy --role-name $R --policy-arn $p; done
aws iam list-attached-role-policies --role-name $R --query 'AttachedPolicies[].PolicyName' --output text
