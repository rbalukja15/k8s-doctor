# CLI

```console
$ k8s-doctor check --snapshot snapshot.json            # or simply: k8s-doctor --snapshot snapshot.json
$ k8s-doctor check --snapshot snapshot.json --format json > report.json
$ k8s-doctor check --snapshot snapshot.json --report report.json --fail-on medium
$ k8s-doctor rules                                      # list built-in rules
$ k8s-doctor schema                                     # JSON Schema of the report
```

`--cluster` (#7) and `--manifests <dir>` (#8) are accepted but exit with an error until their collectors land.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--snapshot <file>` | | Analyze a snapshot JSON file ([ADR-0002](adr/0002-collector-and-snapshot.md)). |
| `--cluster` | | Analyze the current kubeconfig context, read-only (#7). |
| `--manifests <dir>` | | Analyze a manifests folder or kustomize overlay (#8). |
| `--config <file>` | `./.k8s-doctor.yaml` if present | Rule toggles and severity overrides ([rules](rules.md#configuring-rules)). |
| `--enable-rule`, `--disable-rule` | | Rule IDs, repeatable or comma-separated. Applied on top of the config file. Naming a rule in both is an error. |
| `--format` | `text` | `text` or `json`. |
| `--report <file>` | | Also write the JSON report to a file, whatever `--format` is. |
| `--fail-on` | `high` | Lowest severity that makes the exit code non-zero: `critical`, `high`, `medium`, `low` or `none`. |
| `--color` | `auto` | `auto` (only on a terminal, and never when `NO_COLOR` is set), `always` or `never`. |
| `--explain` | off | Print each finding's explanation under it in text output. |
| `--no-llm` | off | Never call an LLM. Until the LLM layer lands (#26) every run behaves this way, and the report's `input.llm` is `false`. |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | No findings at or above `--fail-on`. |
| 1 | Error: unreadable input or config, unknown rule ID, failed write. |
| 2 | Usage error: bad flag value, no input or more than one. |
| 10, 11, 12, 13 | Findings at or above `--fail-on`; the worst is low, medium, high or critical. |

Codes 10–13 are distinct from 1 and 2 so a CI job can tell "the tool failed" from "the tool found problems". A typical gate:

```yaml
- run: k8s-doctor check --snapshot snapshot.json --report k8s-doctor.json --fail-on high
```

## JSON report

The JSON report is the single source of truth: the terminal output, the HTML report (#19) and the eval harness (#21) are all rendered from it. It is validated by [`internal/report/schema/report.v1.json`](../internal/report/schema/report.v1.json) (JSON Schema draft 2020-12), which `k8s-doctor schema` prints.

| Field | Contents |
|---|---|
| `schemaVersion` | `"v1"`. Additive changes keep it; removals or renames bump it. |
| `tool` | Name and version. |
| `generatedAt` | UTC timestamp. Everything else is deterministic for the same input. |
| `input` | Mode, cluster context or path, overlay name, and whether the LLM was used. |
| `summary` | Total, counts for all four severities, worst severity, rules run and not run. |
| `findings` | Most severe first. Each has its stable `id`, `ruleId`, `severity`, `resource`, `fieldPath`, `message`, `evidence`, `category` and an `explanation` with `source` `rule` or `llm`. |
| `groups` | Root-cause groups, each listing its `findingIds` (#17). Empty for now. |
| `rules` | Every registered rule with its effective severity and `status` `run` or `not-run`, with a `reason` when not run. |
