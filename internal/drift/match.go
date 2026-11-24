// Package drift compares two kustomize overlays of the same application:
// two environments (staging and prod), or two tenants in one environment.
// This file matches objects across the overlays; comparing matched objects
// field by field is #29.
package drift

import (
	"fmt"
	"slices"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// RuleResourceMissing is the rule ID of an object that exists in one
// overlay only.
const RuleResourceMissing = "drift-resource-missing"

// overlayNamespace is the identity name of the Namespace object an overlay
// sets with its namespace field.
const overlayNamespace = "(overlay namespace)"

// Identity is what makes two objects in different overlays the same
// object: their kind and the name they have in their source files, before
// kustomize added prefixes, suffixes, generator hashes and a namespace.
type Identity struct {
	Kind string
	Name string
	// Namespace is set only when two objects in one overlay share a kind
	// and source name, which a namespace change would otherwise merge.
	Namespace string
}

func (i Identity) String() string {
	if i.Namespace != "" {
		return i.Kind + "/" + i.Namespace + "/" + i.Name
	}
	return i.Kind + "/" + i.Name
}

// Pair is one object found in both overlays.
type Pair struct {
	Identity Identity
	Base     snapshot.ResourceRef
	Target   snapshot.ResourceRef
}

// Matches is the result of matching two overlays.
type Matches struct {
	Pairs        []Pair
	OnlyInBase   []Object
	OnlyInTarget []Object
}

// Object is an object found in one overlay only.
type Object struct {
	Identity Identity
	Ref      snapshot.ResourceRef
}

// Match pairs the objects of two manifest snapshots by identity.
func Match(base, target *snapshot.Snapshot) Matches {
	b, t := identities(base), identities(target)
	var m Matches
	for _, id := range sortedKeys(b) {
		if ref, ok := t[id]; ok {
			m.Pairs = append(m.Pairs, Pair{Identity: id, Base: b[id], Target: ref})
		} else {
			m.OnlyInBase = append(m.OnlyInBase, Object{Identity: id, Ref: b[id]})
		}
	}
	for _, id := range sortedKeys(t) {
		if _, ok := b[id]; !ok {
			m.OnlyInTarget = append(m.OnlyInTarget, Object{Identity: id, Ref: t[id]})
		}
	}
	return m
}

// Findings reports every object that exists in one overlay only, against
// the overlay that has it. baseName and targetName label the overlays.
func (m Matches) Findings(baseName, targetName string) []rules.Finding {
	var out []rules.Finding
	add := func(o Object, in, missing string) {
		out = append(out, rules.Finding{
			ID:        rules.FindingID(RuleResourceMissing, o.Ref, "metadata.name"),
			RuleID:    RuleResourceMissing,
			Severity:  rules.SeverityMedium,
			Resource:  o.Ref,
			FieldPath: "metadata.name",
			Message:   fmt.Sprintf("%s %s exists only in %s, not in %s", o.Identity.Kind, o.Identity.Name, in, missing),
			Evidence:  map[string]string{"identity": o.Identity.String(), "onlyIn": in, "missingFrom": missing},
		})
	}
	for _, o := range m.OnlyInBase {
		add(o, baseName, targetName)
	}
	for _, o := range m.OnlyInTarget {
		add(o, targetName, baseName)
	}
	return out
}

// identities indexes every object in a snapshot by identity.
func identities(s *snapshot.Snapshot) map[Identity]snapshot.ResourceRef {
	var origins map[string]string
	if s.Source.Overlay != nil {
		origins = s.Source.Overlay.Origins
	}
	var namespace string
	if s.Source.Overlay != nil {
		namespace = s.Source.Overlay.Namespace
	}
	refs := objectRefs(&s.Spec)
	count := map[Identity]int{}
	ids := make([]Identity, len(refs))
	for i, r := range refs {
		name := r.Name
		if o, ok := origins[r.String()]; ok {
			name = o
		}
		if r.Kind == "Namespace" && namespace != "" && r.Name == namespace {
			// The overlay's own namespace: the same object in every overlay
			// whatever it is called.
			name = overlayNamespace
		}
		ids[i] = Identity{Kind: r.Kind, Name: name}
		count[ids[i]]++
	}
	out := make(map[Identity]snapshot.ResourceRef, len(refs))
	for i, r := range refs {
		id := ids[i]
		if count[id] > 1 {
			id.Namespace = r.Namespace
		}
		out[id] = r
	}
	return out
}

// objectRefs lists every object the snapshot models.
func objectRefs(s *snapshot.Spec) []snapshot.ResourceRef {
	var out []snapshot.ResourceRef
	meta := func(kind string, m snapshot.ObjectMeta) {
		out = append(out, snapshot.ResourceRef{Kind: kind, Namespace: m.Namespace, Name: m.Name})
	}
	for i := range s.Namespaces {
		out = append(out, snapshot.ResourceRef{Kind: "Namespace", Name: s.Namespaces[i].Name})
	}
	for i := range s.Workloads {
		out = append(out, s.Workloads[i].Ref())
	}
	for i := range s.Services {
		meta("Service", s.Services[i].ObjectMeta)
	}
	for i := range s.PodDisruptionBudgets {
		meta("PodDisruptionBudget", s.PodDisruptionBudgets[i].ObjectMeta)
	}
	for i := range s.HorizontalPodAutoscalers {
		meta("HorizontalPodAutoscaler", s.HorizontalPodAutoscalers[i].ObjectMeta)
	}
	for i := range s.ConfigMaps {
		meta("ConfigMap", s.ConfigMaps[i].ObjectMeta)
	}
	for i := range s.Secrets {
		meta("Secret", s.Secrets[i].ObjectMeta)
	}
	return out
}

func sortedKeys(m map[Identity]snapshot.ResourceRef) []Identity {
	keys := make([]Identity, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b Identity) int {
		switch {
		case a.String() < b.String():
			return -1
		case a.String() > b.String():
			return 1
		}
		return 0
	})
	return keys
}
