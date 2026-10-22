package checks

import (
	"strings"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// resourceRequestsMissing flags containers without CPU or memory requests.
// A limit without a request counts as a request, because Kubernetes copies
// the limit into the request in that case.
type resourceRequestsMissing struct{ meta }

func newResourceRequestsMissing() rules.Rule {
	return resourceRequestsMissing{meta{
		id:       "resources-requests-missing",
		category: CategoryResources,
		severity: rules.SeverityMedium,
		modes:    rules.AllModes,
		description: "The container does not request CPU and/or memory. The scheduler then places it as if it " +
			"needed nothing, so nodes get overcommitted, the pod is first in line for eviction (BestEffort QoS), " +
			"and HPAs that scale on utilization cannot compute it.",
	}}
}

func (resourceRequestsMissing) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, nil, func(w *snapshot.Workload, c *snapshot.Container) {
		var missing []string
		for _, res := range []string{"cpu", "memory"} {
			if !hasQuantity(c.Resources.Requests, res) && !hasQuantity(c.Resources.Limits, res) {
				missing = append(missing, res)
			}
		}
		if len(missing) == 0 {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".resources.requests",
			Message:   "container " + c.Name + " has no " + strings.Join(missing, " or ") + " request",
			Evidence:  map[string]string{"container": c.Name, "missing": strings.Join(missing, ",")},
		})
	})
	return out
}

// memoryLimitMissing flags containers without a memory limit. CPU limits are
// deliberately not required: exceeding one only throttles the container,
// while an unbounded memory footprint can push the whole node into OOM.
type memoryLimitMissing struct{ meta }

func newMemoryLimitMissing() rules.Rule {
	return memoryLimitMissing{meta{
		id:       "resources-memory-limit-missing",
		category: CategoryResources,
		severity: rules.SeverityMedium,
		modes:    rules.AllModes,
		description: "The container has no memory limit. A leak or a traffic spike can grow it until the node " +
			"runs out of memory and the kernel kills other pods on the same node. CPU limits are not required " +
			"by this rule, because exceeding them only throttles the container.",
	}}
}

func (memoryLimitMissing) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, nil, func(w *snapshot.Workload, c *snapshot.Container) {
		if hasQuantity(c.Resources.Limits, "memory") {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".resources.limits.memory",
			Message:   "container " + c.Name + " has no memory limit",
			Evidence:  map[string]string{"container": c.Name},
		})
	})
	return out
}

func hasQuantity(m map[string]string, key string) bool {
	return strings.TrimSpace(m[key]) != ""
}
