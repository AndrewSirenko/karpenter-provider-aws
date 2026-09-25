#!/usr/bin/env bash
set -euo pipefail

# Bump the coupled Kubernetes library cluster (k8s.io/*, sigs.k8s.io/karpenter,
# and the transitive controller-runtime / k8s.io deps it pins) to a target
# Kubernetes minor version, across every go.mod in the repo.
#
# Dependabot intentionally ignores minor/major bumps of these modules (see
# .github/dependabot.yaml) because they must move in lockstep with the target
# Kubernetes minor. This script is the manual, reproducible way to do that bump.
#
# See hack/RUNBOOK-kubernetes-version-bump.md for the full procedure.
#
# Usage:
#   hack/bump-kubernetes.sh <k8s-minor>
# Example (target Kubernetes 1.34):
#   hack/bump-kubernetes.sh 1.34

usage() {
  echo "Usage: $0 <k8s-minor>   e.g. $0 1.34" >&2
  exit 1
}

[[ $# -eq 1 ]] || usage
K8S_MINOR="$1"
# Accept "1.34" and normalise; k8s.io staging modules are published as v0.<minor>.<patch>.
[[ "$K8S_MINOR" =~ ^1\.[0-9]+$ ]] || { echo "error: expected a Kubernetes minor like 1.34, got '$K8S_MINOR'" >&2; usage; }
STAGING_VER="v0.${K8S_MINOR#1.}" # 1.34 -> v0.34

# Match the Makefile's MOD_DIRS (every go.mod except the website).
mapfile -t MOD_DIRS < <(find . -path "./website" -prune -o -name go.mod -type f -print0 | xargs -0 -n1 dirname | sort -u)

echo ">> Target Kubernetes minor: ${K8S_MINOR}  (k8s.io/* staging version: ${STAGING_VER})"
echo ">> Module directories: ${MOD_DIRS[*]}"

repo_root="$(pwd)"
for dir in "${MOD_DIRS[@]}"; do
  echo
  echo "==> ${dir}"
  cd "${repo_root}/${dir}"

  # 1. Take the latest sigs.k8s.io/karpenter (upstream, which should already target
  #    the new Kubernetes minor). This drives the compatible controller-runtime and
  #    transitive k8s.io versions via its go.mod.
  if go list -m sigs.k8s.io/karpenter >/dev/null 2>&1; then
    echo "   go get -u sigs.k8s.io/karpenter@HEAD"
    go get -u sigs.k8s.io/karpenter@HEAD
  fi

  # 2. Pin the direct k8s.io/* staging modules to the target minor (go resolves the
  #    latest patch of ${STAGING_VER}).
  while read -r mod; do
    [[ -n "$mod" ]] || continue
    echo "   go get ${mod}@${STAGING_VER}"
    go get "${mod}@${STAGING_VER}"
  done < <(go mod edit -json | python3 -c '
import json,sys
m=json.load(sys.stdin)
for r in (m.get("Require") or []):
    p=r["Path"]
    if p.startswith("k8s.io/") and not r.get("Indirect"):
        print(p)
')

  # 3. Reconcile.
  go mod tidy
done

cd "${repo_root}"
echo
echo ">> Done. Next steps (see hack/RUNBOOK-kubernetes-version-bump.md):"
echo "   1. make verify test"
echo "   2. Open a PR and run the snapshot e2e suite against it before merging."
