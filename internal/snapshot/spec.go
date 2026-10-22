package snapshot

// Workload kinds the snapshot understands.
const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindDaemonSet   = "DaemonSet"
	KindJob         = "Job"
	KindCronJob     = "CronJob"
)

var workloadKinds = map[string]bool{
	KindDeployment: true, KindStatefulSet: true, KindDaemonSet: true, KindJob: true, KindCronJob: true,
}

// Spec is the desired-state section, present in both modes.
type Spec struct {
	Namespaces               []Namespace               `json:"namespaces,omitempty"`
	Workloads                []Workload                `json:"workloads,omitempty"`
	Services                 []Service                 `json:"services,omitempty"`
	PodDisruptionBudgets     []PodDisruptionBudget     `json:"podDisruptionBudgets,omitempty"`
	HorizontalPodAutoscalers []HorizontalPodAutoscaler `json:"horizontalPodAutoscalers,omitempty"`
	ConfigMaps               []ConfigRef               `json:"configMaps,omitempty"`
	Secrets                  []ConfigRef               `json:"secrets,omitempty"`
}

// Namespace is a cluster-scoped Namespace object.
type Namespace struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
}

// Workload is a Deployment, StatefulSet, DaemonSet, Job or CronJob,
// flattened so rules can treat them uniformly.
type Workload struct {
	Kind       string `json:"kind"`
	APIVersion string `json:"apiVersion"`
	ObjectMeta
	// Replicas is nil when the field is unset (DaemonSet, Job, CronJob, or an
	// omitted field, which Kubernetes defaults to 1 for Deployments and
	// StatefulSets).
	Replicas *int32 `json:"replicas,omitempty"`
	// Selector is spec.selector.matchLabels.
	Selector map[string]string `json:"selector,omitempty"`
	Template PodTemplate       `json:"template"`
}

// Ref returns the workload's resource reference.
func (w *Workload) Ref() ResourceRef {
	return ResourceRef{Kind: w.Kind, Namespace: w.Namespace, Name: w.Name}
}

// PodSpecPath is the manifest field path of the pod spec for this kind,
// used to build finding field paths that point into the real manifest.
func (w *Workload) PodSpecPath() string {
	if w.Kind == KindCronJob {
		return "spec.jobTemplate.spec.template.spec"
	}
	return "spec.template.spec"
}

// EffectiveReplicas returns the replica count Kubernetes would run, and false
// for kinds that have no replica count.
func (w *Workload) EffectiveReplicas() (int32, bool) {
	switch w.Kind {
	case KindDeployment, KindStatefulSet:
		if w.Replicas == nil {
			return 1, true
		}
		return *w.Replicas, true
	default:
		return 0, false
	}
}

// IsLongRunning reports whether the workload serves traffic or runs
// continuously, as opposed to running to completion.
func (w *Workload) IsLongRunning() bool {
	switch w.Kind {
	case KindDeployment, KindStatefulSet, KindDaemonSet:
		return true
	default:
		return false
	}
}

// PodTemplate is the pod template of a workload.
type PodTemplate struct {
	Labels map[string]string `json:"labels,omitempty"`
	Spec   PodSpec           `json:"spec"`
}

// PodSpec keeps the pod fields that rules inspect.
type PodSpec struct {
	ServiceAccountName        string                     `json:"serviceAccountName,omitempty"`
	HostNetwork               bool                       `json:"hostNetwork,omitempty"`
	HostPID                   bool                       `json:"hostPID,omitempty"`
	HostIPC                   bool                       `json:"hostIPC,omitempty"`
	SecurityContext           *PodSecurityContext        `json:"securityContext,omitempty"`
	Affinity                  *Affinity                  `json:"affinity,omitempty"`
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	Volumes                   []Volume                   `json:"volumes,omitempty"`
	InitContainers            []Container                `json:"initContainers,omitempty"`
	Containers                []Container                `json:"containers"`
}

// PodSecurityContext is the pod-level security context.
type PodSecurityContext struct {
	RunAsNonRoot   *bool           `json:"runAsNonRoot,omitempty"`
	RunAsUser      *int64          `json:"runAsUser,omitempty"`
	SeccompProfile *SeccompProfile `json:"seccompProfile,omitempty"`
}

// SeccompProfile is a pod or container seccomp profile.
type SeccompProfile struct {
	Type string `json:"type"`
}

// Affinity keeps whether pod anti-affinity is declared and its topology keys.
type Affinity struct {
	PodAntiAffinity *PodAntiAffinity `json:"podAntiAffinity,omitempty"`
}

// PodAntiAffinity lists the topology keys used by required and preferred terms.
type PodAntiAffinity struct {
	RequiredTopologyKeys  []string `json:"requiredTopologyKeys,omitempty"`
	PreferredTopologyKeys []string `json:"preferredTopologyKeys,omitempty"`
}

// TopologySpreadConstraint is a pod topology spread constraint.
type TopologySpreadConstraint struct {
	TopologyKey       string `json:"topologyKey"`
	MaxSkew           int32  `json:"maxSkew"`
	WhenUnsatisfiable string `json:"whenUnsatisfiable,omitempty"`
}

// Volume keeps the volume name and its source type (e.g. "hostPath",
// "configMap", "secret", "emptyDir"), plus the host path when relevant.
type Volume struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	HostPath string `json:"hostPath,omitempty"`
	// Source names the ConfigMap, Secret or PVC the volume refers to.
	Source string `json:"source,omitempty"`
}

// Container keeps the container fields that rules inspect. Env values are
// never recorded, only the variable names and where they come from.
type Container struct {
	Name            string                    `json:"name"`
	Image           string                    `json:"image"`
	ImagePullPolicy string                    `json:"imagePullPolicy,omitempty"`
	Resources       Resources                 `json:"resources,omitzero"`
	ReadinessProbe  *Probe                    `json:"readinessProbe,omitempty"`
	LivenessProbe   *Probe                    `json:"livenessProbe,omitempty"`
	StartupProbe    *Probe                    `json:"startupProbe,omitempty"`
	SecurityContext *ContainerSecurityContext `json:"securityContext,omitempty"`
	Ports           []ContainerPort           `json:"ports,omitempty"`
	Env             []EnvVar                  `json:"env,omitempty"`
}

// Resources holds requests and limits as Kubernetes quantity strings.
type Resources struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// Probe is a readiness, liveness or startup probe.
type Probe struct {
	// Handler is one of "httpGet", "tcpSocket", "exec" or "grpc".
	Handler             string `json:"handler"`
	Path                string `json:"path,omitempty"`
	Port                string `json:"port,omitempty"`
	InitialDelaySeconds int32  `json:"initialDelaySeconds,omitempty"`
	PeriodSeconds       int32  `json:"periodSeconds,omitempty"`
	TimeoutSeconds      int32  `json:"timeoutSeconds,omitempty"`
	FailureThreshold    int32  `json:"failureThreshold,omitempty"`
}

// ContainerSecurityContext is the container-level security context.
type ContainerSecurityContext struct {
	RunAsNonRoot             *bool         `json:"runAsNonRoot,omitempty"`
	RunAsUser                *int64        `json:"runAsUser,omitempty"`
	Privileged               *bool         `json:"privileged,omitempty"`
	AllowPrivilegeEscalation *bool         `json:"allowPrivilegeEscalation,omitempty"`
	ReadOnlyRootFilesystem   *bool         `json:"readOnlyRootFilesystem,omitempty"`
	Capabilities             *Capabilities `json:"capabilities,omitempty"`
}

// Capabilities lists added and dropped Linux capabilities.
type Capabilities struct {
	Add  []string `json:"add,omitempty"`
	Drop []string `json:"drop,omitempty"`
}

// ContainerPort is a declared container port.
type ContainerPort struct {
	Name          string `json:"name,omitempty"`
	ContainerPort int32  `json:"containerPort"`
	Protocol      string `json:"protocol,omitempty"`
}

// EnvVar records an environment variable name and the kind of its source
// ("value", "configMapKeyRef", "secretKeyRef", "fieldRef", ...). Literal
// values are dropped at collection time so they can never reach a report or
// an LLM prompt.
type EnvVar struct {
	Name       string `json:"name"`
	SourceType string `json:"sourceType"`
}

// Service is a Kubernetes Service.
type Service struct {
	ObjectMeta
	Type     string            `json:"type,omitempty"`
	Selector map[string]string `json:"selector,omitempty"`
	Ports    []ServicePort     `json:"ports,omitempty"`
}

// ServicePort is a Service port.
type ServicePort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort,omitempty"`
	Protocol   string `json:"protocol,omitempty"`
}

// PodDisruptionBudget is a policy/v1 PodDisruptionBudget. MinAvailable and
// MaxUnavailable keep the int-or-percent string form ("2", "50%").
type PodDisruptionBudget struct {
	APIVersion string `json:"apiVersion"`
	ObjectMeta
	Selector       map[string]string `json:"selector,omitempty"`
	MinAvailable   string            `json:"minAvailable,omitempty"`
	MaxUnavailable string            `json:"maxUnavailable,omitempty"`
}

// HorizontalPodAutoscaler is an autoscaling HPA.
type HorizontalPodAutoscaler struct {
	APIVersion string `json:"apiVersion"`
	ObjectMeta
	TargetRef   ScaleTargetRef `json:"targetRef"`
	MinReplicas *int32         `json:"minReplicas,omitempty"`
	MaxReplicas int32          `json:"maxReplicas"`
	Metrics     []HPAMetric    `json:"metrics,omitempty"`
}

// ScaleTargetRef is the workload an HPA scales, in the HPA's namespace.
type ScaleTargetRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

// HPAMetric is one HPA metric. For resource metrics, Resource is "cpu" or
// "memory" and Target is "Utilization" or "AverageValue".
type HPAMetric struct {
	Type     string `json:"type"`
	Resource string `json:"resource,omitempty"`
	Target   string `json:"target,omitempty"`
}

// ConfigRef is a ConfigMap or Secret recorded by name and key names only.
type ConfigRef struct {
	ObjectMeta
	Type string   `json:"type,omitempty"`
	Keys []string `json:"keys,omitempty"`
}

// FindHPAFor returns the HPA that targets the workload, if any.
func (s *Spec) FindHPAFor(w *Workload) *HorizontalPodAutoscaler {
	for i := range s.HorizontalPodAutoscalers {
		h := &s.HorizontalPodAutoscalers[i]
		if h.Namespace == w.Namespace && h.TargetRef.Kind == w.Kind && h.TargetRef.Name == w.Name {
			return h
		}
	}
	return nil
}
