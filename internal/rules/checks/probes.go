package checks

import (
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// readinessProbeMissing flags long-running containers without a readiness
// probe. Jobs and CronJobs run to completion and receive no traffic, so they
// are skipped.
type readinessProbeMissing struct{ meta }

func newReadinessProbeMissing() rules.Rule {
	return readinessProbeMissing{meta{
		id:       "probe-readiness-missing",
		category: CategoryProbes,
		severity: rules.SeverityMedium,
		modes:    rules.AllModes,
		description: "The container has no readiness probe, so it receives Service traffic as soon as the process " +
			"starts, before it can serve requests, and keeps receiving it while it is overloaded or broken. " +
			"Rollouts also proceed without waiting for new pods to become healthy.",
	}}
}

func (readinessProbeMissing) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, (*snapshot.Workload).IsLongRunning, func(w *snapshot.Workload, c *snapshot.Container) {
		if c.ReadinessProbe != nil {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".readinessProbe",
			Message:   "container " + c.Name + " has no readiness probe",
			Evidence:  map[string]string{"container": c.Name},
		})
	})
	return out
}

// livenessProbeMissing flags long-running containers without a liveness
// probe. It is low severity: a bad liveness probe causes more outages than a
// missing one, but a deadlocked process with no probe is never restarted.
type livenessProbeMissing struct{ meta }

func newLivenessProbeMissing() rules.Rule {
	return livenessProbeMissing{meta{
		id:       "probe-liveness-missing",
		category: CategoryProbes,
		severity: rules.SeverityLow,
		modes:    rules.AllModes,
		description: "The container has no liveness probe. If the process deadlocks or hangs without exiting, " +
			"Kubernetes never restarts it. Keep the probe cheap and independent of downstream dependencies, " +
			"or it will restart healthy pods during an unrelated outage.",
	}}
}

func (livenessProbeMissing) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, (*snapshot.Workload).IsLongRunning, func(w *snapshot.Workload, c *snapshot.Container) {
		if c.LivenessProbe != nil {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".livenessProbe",
			Message:   "container " + c.Name + " has no liveness probe",
			Evidence:  map[string]string{"container": c.Name},
		})
	})
	return out
}
