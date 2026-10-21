// Package collector turns a live cluster (#7) or a folder of manifests and
// kustomize overlays (#8) into a snapshot.Snapshot. Collectors are
// read-only: they only ever get, list and watch, and never read Secret values.
package collector
