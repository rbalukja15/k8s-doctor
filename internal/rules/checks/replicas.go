package checks

import (
	"strconv"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// singleReplica flags Deployments that run exactly one replica. StatefulSets
// are skipped because a single replica is often deliberate there (a primary
// database), and Deployments scaled to zero are treated as intentionally off.
// An HPA with minReplicas >= 2 takes over the replica count, so it clears
// the finding.
type singleReplica struct{ meta }

func newSingleReplica() rules.Rule {
	return singleReplica{meta{
		id:       "deployment-single-replica",
		category: CategoryAvailability,
		severity: rules.SeverityMedium,
		modes:    rules.AllModes,
		description: "The Deployment runs a single replica, so every rollout, node drain, eviction or crash " +
			"takes the workload fully down until a replacement pod is ready.",
	}}
}

func (singleReplica) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	for i := range s.Spec.Workloads {
		w := &s.Spec.Workloads[i]
		if w.Kind != snapshot.KindDeployment {
			continue
		}
		replicas, _ := w.EffectiveReplicas()
		if replicas != 1 {
			continue
		}
		evidence := map[string]string{"replicas": "1"}
		if w.Replicas == nil {
			evidence["replicas"] = "unset (defaults to 1)"
		}
		if hpa := s.Spec.FindHPAFor(w); hpa != nil {
			minReplicas := int32(1)
			if hpa.MinReplicas != nil {
				minReplicas = *hpa.MinReplicas
			}
			if minReplicas >= 2 {
				continue
			}
			evidence["hpa"] = hpa.Name
			evidence["hpaMinReplicas"] = strconv.Itoa(int(minReplicas))
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: "spec.replicas",
			Message:   "Deployment " + w.Name + " runs a single replica",
			Evidence:  evidence,
		})
	}
	return out
}
