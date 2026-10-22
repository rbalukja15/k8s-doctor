package rules

import "github.com/rbalukja15/k8s-doctor/internal/snapshot"

// ContainerPath returns the manifest field path of a container in a
// workload, addressing it by name: spec.template.spec.containers[name=api].
func ContainerPath(w *snapshot.Workload, c *snapshot.Container) string {
	return w.PodSpecPath() + ".containers[name=" + c.Name + "]"
}

// PodSpecField returns the manifest field path of a pod spec field.
func PodSpecField(w *snapshot.Workload, field string) string {
	return w.PodSpecPath() + "." + field
}
