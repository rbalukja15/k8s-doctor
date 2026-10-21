# Devlog

Short notes on what was built and what was learned, newest first.

## 2026-10-21: Foundations

- Scaffolded the Go module, Makefile, lint (golangci-lint v2 with gofumpt/goimports), pre-commit hooks and CI.
- Wrote ADR-0001 (Go, read-only, rules find and the LLM explains, LLM data policy) and ADR-0002 (snapshot schema split into spec and runtime).
- Built the rule engine: severity model, stable finding IDs, mode-aware rules with an explicit "not run" list, config file with rule toggles and severity overrides.
- Defined the fixture format and a runner that diffs actual against expected findings, plus a test that fails if any rule lacks positive, clean and near-miss fixtures.
- First rule batch: resource requests, memory limits, readiness and liveness probes, mutable image tags, stale pull policy, single-replica Deployments.

Decisions worth remembering:

- A limit without a request is not flagged as a missing request, because Kubernetes copies the limit into the request.
- CPU limits are deliberately not required; only memory limits are, since exceeding a CPU limit throttles while unbounded memory can take down the node.
- `imagePullPolicy` is only flagged when it is explicitly `IfNotPresent`/`Never` on a mutable tag. Left unset, Kubernetes defaults it to `Always` for `:latest`.
