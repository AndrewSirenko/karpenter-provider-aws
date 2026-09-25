# Runbook: bumping to a new Kubernetes version

Karpenter's Kubernetes library cluster — `k8s.io/*`, `sigs.k8s.io/*` (including
`sigs.k8s.io/karpenter`), `knative.dev/*`, and the `controller-runtime` version
they pull in — is **coupled to the Kubernetes minor we target**. These modules
must move together; bumping one independently breaks compilation or, worse,
silently changes API behavior.

For that reason Dependabot is configured to **ignore minor/major updates** of
these modules (`.github/dependabot.yaml`). It still opens patch-level PRs for
them, but the minor bump is a deliberate, manual step done via this runbook.

## When to run

- A new Kubernetes minor reaches GA and we want to build/test against it.
- Upstream `kubernetes-sigs/karpenter` has moved to the new minor (we consume it
  via the pinned `sigs.k8s.io/karpenter` pseudo-version).

Patch-level bumps do **not** need this runbook — Dependabot handles those on the
normal weekly (Saturday) schedule.

## Procedure

1. **Bump everything in lockstep:**
   ```
   make bump-kubernetes K8S_MINOR=1.34
   ```
   This runs `hack/bump-kubernetes.sh`, which for every `go.mod` in the repo:
   - takes the latest `sigs.k8s.io/karpenter@HEAD` (drives the compatible
     `controller-runtime` and transitive `k8s.io` versions),
   - pins the direct `k8s.io/*` staging modules to `v0.<minor>` (e.g. `v0.34`),
   - runs `go mod tidy`.

2. **Verify locally:**
   ```
   make verify test
   ```
   Fix any API drift (deprecated/removed Kubernetes APIs are the usual source).

3. **Open a PR** with the changes.

4. **Run the snapshot e2e suite against the PR** before merging — a repository
   member comments `snapshot` on the PR, which publishes a snapshot image and
   runs the E2E matrix against that commit. Dependency-bump PRs are also picked
   up automatically by the Sunday run
   (`.github/workflows/dependabot-e2e.yaml`), but for a manual minor bump trigger
   it explicitly and confirm the full matrix is green.

5. **Merge** once e2e is green and a member has approved.

## Notes

- `make update-karpenter` is the lighter-weight cousin: it only bumps
  `sigs.k8s.io/karpenter@HEAD` + `go mod tidy`, for taking in upstream changes
  within the *same* Kubernetes minor. Use `make bump-kubernetes` when changing
  the Kubernetes minor itself.
- If the script leaves a `k8s.io/*` module on an unexpected version, check
  whether it is a direct dependency (`go mod edit -json`) — indirect ones are
  reconciled by `go mod tidy` from `sigs.k8s.io/karpenter`'s requirements.
- The list of supported e2e `k8s_version` values lives in
  `.github/workflows/e2e-matrix.yaml`; add the new minor there as part of the bump.
