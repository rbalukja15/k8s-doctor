// Package snapshot defines the normalized model that both collectors produce
// and every rule consumes. The design is recorded in docs/adr/0002-collector-and-snapshot.md.
//
// A snapshot has three parts:
//   - Spec: desired state. Present in both modes and the only part that the
//     cluster and manifest collectors must agree on.
//   - Runtime: observed state (pod status, restarts, events). Cluster mode only.
//   - Source: where the snapshot came from, including kustomize overlay
//     metadata in manifest mode.
//
// Secret values never enter a snapshot: Secrets and ConfigMaps are recorded
// by name and key only.
package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// SchemaVersion is the version of the snapshot JSON format. Bump it on any
// incompatible change to the types in this package.
const SchemaVersion = "v1alpha1"

// Mode says which collector produced a snapshot.
type Mode string

const (
	// ModeManifest snapshots come from manifest folders or kustomize overlays.
	ModeManifest Mode = "manifest"
	// ModeCluster snapshots come from a live cluster through the read-only collector.
	ModeCluster Mode = "cluster"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool { return m == ModeManifest || m == ModeCluster }

// Snapshot is the normalized input to the rule engine.
type Snapshot struct {
	SchemaVersion string   `json:"schemaVersion"`
	Mode          Mode     `json:"mode"`
	Source        Source   `json:"source"`
	Spec          Spec     `json:"spec"`
	Runtime       *Runtime `json:"runtime,omitempty"`
}

// Source records where a snapshot was collected from.
type Source struct {
	// Cluster is the kubeconfig context name (cluster mode).
	Cluster string `json:"cluster,omitempty"`
	// Path is the manifest folder or overlay directory (manifest mode).
	Path string `json:"path,omitempty"`
	// Overlay carries kustomize metadata used to match resources across
	// overlays for drift detection (manifest mode).
	Overlay *Overlay `json:"overlay,omitempty"`
}

// Overlay is the subset of a kustomization that changes resource identity.
type Overlay struct {
	Name       string `json:"name"`
	NamePrefix string `json:"namePrefix,omitempty"`
	NameSuffix string `json:"nameSuffix,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
}

// Validate checks the invariants every consumer relies on.
func (s *Snapshot) Validate() error {
	var errs []error
	if s.SchemaVersion != SchemaVersion {
		errs = append(errs, fmt.Errorf("unsupported schemaVersion %q (want %q)", s.SchemaVersion, SchemaVersion))
	}
	if !s.Mode.Valid() {
		errs = append(errs, fmt.Errorf("invalid mode %q (want %q or %q)", s.Mode, ModeManifest, ModeCluster))
	}
	if s.Mode == ModeManifest && s.Runtime != nil {
		errs = append(errs, errors.New("runtime section must be absent in manifest mode"))
	}
	if s.Mode == ModeCluster && s.Runtime == nil {
		errs = append(errs, errors.New("runtime section is required in cluster mode"))
	}
	for i := range s.Spec.Workloads {
		w := &s.Spec.Workloads[i]
		if !workloadKinds[w.Kind] {
			errs = append(errs, fmt.Errorf("spec.workloads[%d]: unsupported kind %q", i, w.Kind))
		}
		if w.Name == "" {
			errs = append(errs, fmt.Errorf("spec.workloads[%d]: name is required", i))
		}
	}
	return errors.Join(errs...)
}

// Decode reads a snapshot from JSON. Unknown fields are rejected so that a
// typo in a fixture fails loudly instead of being ignored.
func Decode(r io.Reader) (*Snapshot, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("invalid snapshot: %w", err)
	}
	return &s, nil
}

// Load reads and validates a snapshot JSON file.
func Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}
