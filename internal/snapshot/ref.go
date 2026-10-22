package snapshot

import (
	"fmt"
	"strings"
)

// ResourceRef identifies a Kubernetes object. Its string form,
// Kind/namespace/name (or Kind/name for cluster-scoped objects), is part of
// every finding ID, so it must stay stable.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

func (r ResourceRef) String() string {
	if r.Namespace == "" {
		return r.Kind + "/" + r.Name
	}
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

// ParseResourceRef parses the String form of a ResourceRef.
func ParseResourceRef(s string) (ResourceRef, error) {
	parts := strings.Split(s, "/")
	for _, p := range parts {
		if p == "" {
			return ResourceRef{}, fmt.Errorf("invalid resource ref %q", s)
		}
	}
	switch len(parts) {
	case 2:
		return ResourceRef{Kind: parts[0], Name: parts[1]}, nil
	case 3:
		return ResourceRef{Kind: parts[0], Namespace: parts[1], Name: parts[2]}, nil
	default:
		return ResourceRef{}, fmt.Errorf("invalid resource ref %q (want Kind/namespace/name or Kind/name)", s)
	}
}

// ObjectMeta is the metadata kept for every namespaced object.
type ObjectMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}
