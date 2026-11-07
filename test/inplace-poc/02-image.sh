#!/usr/bin/env bash
set -euo pipefail
export AWS_PROFILE=poc-dev AWS_DEFAULT_REGION=ca-central-1 AWS_REGION=ca-central-1
export PATH=$HOME/sdk/go1.26.10/bin:$PATH GOTOOLCHAIN=local GOPROXY=direct GOSUMDB=off GOFLAGS=-mod=mod GIT_CONFIG_GLOBAL=/dev/null
ACCT=$(aws sts get-caller-identity --query Account --output text)
REG=$ACCT.dkr.ecr.ca-central-1.amazonaws.com
aws ecr describe-repositories --repository-names karpenter-poc >/dev/null 2>&1 || aws ecr create-repository --repository-name karpenter-poc >/dev/null
ko login $REG --username AWS --password "$(aws ecr get-login-password)" 2>&1 | tail -1
cd /workplace/andsirey/agents/workstreams/ebs-update/poc-provider-aws
IMG=$(KO_DOCKER_REPO=$REG/karpenter-poc ko build --bare --platform linux/amd64 --tags inplace-$(date +%H%M) ./cmd/controller)
echo "$IMG" | tee /workplace/andsirey/agents/workstreams/ebs-update/poc/image.txt
