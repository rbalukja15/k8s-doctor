package report

import _ "embed"

// Schema is the JSON Schema (draft 2020-12) for the JSON report, printed by
// `k8s-doctor schema`.
//
//go:embed schema/report.v1.json
var Schema []byte
