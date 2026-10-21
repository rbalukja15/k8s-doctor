// Package llm is the optional explanation layer (#16, #17, #26). It only
// explains and narrates findings produced by the rule engine; it never
// creates findings. Every prompt passes through redaction first, and the
// whole tool works without it (--no-llm).
package llm
