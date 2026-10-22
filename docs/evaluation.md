# Evaluation

Detection quality is measured, not eyeballed. The harness (#21) will run the rule engine over every labeled fixture in `fixtures/` with `--no-llm` and report:

- precision and recall per rule category, matching findings by rule ID + resource + field path
- false-positive rate on clean and near-miss fixtures
- with `make eval-llm`: the LLM faithfulness pass rate (#16)

## Current state

`make test` already runs every fixture and fails on any missing or unexpected finding, so all built-in rules currently pass their fixtures exactly. Numbers will be published here once `make eval` exists.

| Category | Rules | Fixtures | Precision | Recall |
|---|---|---|---|---|
| resources | 2 | — | — | — |
| probes | 2 | — | — | — |
| images | 2 | — | — | — |
| availability | 1 | — | — | — |
