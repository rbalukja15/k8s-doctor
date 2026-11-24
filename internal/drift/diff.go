package drift

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rbalukja15/k8s-doctor/internal/config"
	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// RuleField is the rule ID of a field that differs between two matched
// objects and is not allowlisted.
const RuleField = "drift-field"

// unset stands for a field one side does not have.
const unset = "(unset)"

// Presets name groups of expected differences. Kinds the snapshot does not
// model yet (Ingress, KongPlugin: #43) match nothing until they land.
var presets = map[string][]string{
	"replicas":              {"spec.replicas"},
	"resources":             {"**.resources.**"},
	"hpaBounds":             {"HorizontalPodAutoscaler:spec.minReplicas", "HorizontalPodAutoscaler:spec.maxReplicas"},
	"ingressClassAndIssuer": {"Ingress:spec.ingressClassName", "Ingress:metadata.annotations[cert-manager.io/cluster-issuer]"},
	"ipAllowlists":          {"KongPlugin:config.allow**", "KongPlugin:config.deny**"},
	// imageTags and hostnames compare values rather than skip paths; see
	// Options.
	"imageTags": nil,
	"hostnames": nil,
}

// DefaultPresets apply when the config does not list presets.
var DefaultPresets = []string{"replicas", "resources", "imageTags", "hostnames", "hpaBounds"}

// Options says which differences are expected.
type Options struct {
	allow []glob
	// imageTags ignores an image difference in tag or digest only.
	imageTags bool
	// hostnames replaces each overlay's name (the tenant or environment,
	// e.g. "acme" or "staging") in string values with a placeholder, so
	// acme.example.com and globex.example.com compare equal.
	hostnames bool
}

// NewOptions builds the options from the drift section of the config.
func NewOptions(cfg config.Drift) (Options, error) {
	names := DefaultPresets
	if cfg.Presets != nil {
		names = *cfg.Presets
	}
	var o Options
	var patterns []string
	for _, n := range names {
		globs, ok := presets[n]
		if !ok {
			return Options{}, fmt.Errorf("drift: unknown preset %q", n)
		}
		patterns = append(patterns, globs...)
		o.imageTags = o.imageTags || n == "imageTags"
		o.hostnames = o.hostnames || n == "hostnames"
	}
	for _, p := range append(patterns, cfg.Allowlist...) {
		g, err := parseGlob(p)
		if err != nil {
			return Options{}, err
		}
		o.allow = append(o.allow, g)
	}
	return o, nil
}

// glob matches a field path, optionally only on one kind. In the pattern,
// * matches within one path segment and ** across segments.
type glob struct {
	kind string
	re   *regexp.Regexp
}

func parseGlob(p string) (glob, error) {
	var g glob
	if kind, rest, ok := strings.Cut(p, ":"); ok && kind != "" && !strings.ContainsAny(kind, ".*[") {
		g.kind, p = kind, rest
	}
	if p == "" {
		return glob{}, fmt.Errorf("drift: empty allowlist pattern")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(p); i++ {
		switch {
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i++
		case p[i] == '*':
			b.WriteString(`[^.\[]*`)
		default:
			b.WriteString(regexp.QuoteMeta(p[i : i+1]))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return glob{}, fmt.Errorf("drift: pattern %q: %w", p, err)
	}
	g.re = re
	return g, nil
}

func (o Options) allowed(kind, path, base, target string) bool {
	for _, g := range o.allow {
		if (g.kind == "" || g.kind == kind) && g.re.MatchString(path) {
			return true
		}
	}
	if o.imageTags && strings.HasSuffix(path, ".image") && imageRepo(base) == imageRepo(target) {
		return true
	}
	return false
}

// imageRepo strips the tag and digest from an image reference. A registry
// port is not a tag: only a colon after the last slash starts one.
func imageRepo(image string) string {
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		image = image[:i]
	}
	return image
}

// Diff matches the objects of two overlays and reports every difference
// that is not expected: objects present in one overlay only, and fields
// that differ between matched objects. Findings point at the target.
func Diff(base, target *snapshot.Snapshot, o Options) []rules.Finding {
	baseName, targetName := overlayName(base), overlayName(target)
	m := Match(base, target)
	out := m.Findings(baseName, targetName)
	bObj, tObj := objects(base), objects(target)
	bNorm, tNorm := normalizer(base, o.hostnames), normalizer(target, o.hostnames)
	for _, p := range m.Pairs {
		bf := fields(p.Base.Kind, bObj[p.Base.String()], bNorm)
		tf := fields(p.Target.Kind, tObj[p.Target.String()], tNorm)
		for _, path := range unionKeys(bf, tf) {
			bv, tv := valueOr(bf, path), valueOr(tf, path)
			if bv == tv || o.allowed(p.Identity.Kind, path, bv, tv) {
				continue
			}
			out = append(out, rules.Finding{
				ID:        rules.FindingID(RuleField, p.Target, path),
				RuleID:    RuleField,
				Severity:  fieldSeverity(p.Identity.Kind, path),
				Resource:  p.Target,
				FieldPath: path,
				Message:   fmt.Sprintf("%s %s: %s is %s in %s but %s in %s", p.Identity.Kind, p.Identity.Name, path, bv, baseName, tv, targetName),
				Evidence: map[string]string{
					"identity": p.Identity.String(), "base": bv, "target": tv,
					"baseResource": p.Base.String(), "baseOverlay": baseName, "targetOverlay": targetName,
				},
			})
		}
	}
	return out
}

// fieldSeverity ranks security settings above config and env differences,
// and those above everything else.
func fieldSeverity(kind, path string) rules.Severity {
	for _, s := range []string{"securityContext", "privileged", "hostNetwork", "hostPID", "hostIPC", "hostPath", "allowPrivilegeEscalation"} {
		if strings.Contains(path, s) {
			return rules.SeverityHigh
		}
	}
	if strings.Contains(path, ".env[") || ((kind == "ConfigMap" || kind == "Secret") && strings.HasPrefix(path, "data")) {
		return rules.SeverityMedium
	}
	return rules.SeverityLow
}

func overlayName(s *snapshot.Snapshot) string {
	if s.Source.Overlay != nil && s.Source.Overlay.Name != "" {
		return s.Source.Overlay.Name
	}
	return s.Source.Path
}

// normalizer rewrites string values so that what kustomize or the overlay
// changed on purpose compares equal: renamed objects' names go back to
// their source names, the overlay namespace becomes a placeholder, and with
// hostnames the overlay name does too.
func normalizer(s *snapshot.Snapshot, hostnames bool) func(string) string {
	o := s.Source.Overlay
	if o == nil {
		return func(v string) string { return v }
	}
	names := map[string]string{}
	for ref, orig := range o.Origins {
		if r, err := snapshot.ParseResourceRef(ref); err == nil {
			names[r.Name] = orig
		}
	}
	var token *regexp.Regexp
	if hostnames && o.Name != "" {
		token = regexp.MustCompile(`(^|[^a-z0-9])` + regexp.QuoteMeta(o.Name) + `($|[^a-z0-9])`)
	}
	return func(v string) string {
		if orig, ok := names[v]; ok {
			return orig
		}
		if o.Namespace != "" && v == o.Namespace {
			return "{namespace}"
		}
		if token != nil {
			v = token.ReplaceAllString(v, "${1}{overlay}${2}")
		}
		return v
	}
}

// objects indexes the JSON form of every object by ref.
func objects(s *snapshot.Snapshot) map[string]map[string]any {
	out := map[string]map[string]any{}
	add := func(ref snapshot.ResourceRef, v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			return
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			out[ref.String()] = m
		}
	}
	sp := &s.Spec
	for i := range sp.Namespaces {
		add(snapshot.ResourceRef{Kind: "Namespace", Name: sp.Namespaces[i].Name}, sp.Namespaces[i])
	}
	for i := range sp.Workloads {
		add(sp.Workloads[i].Ref(), sp.Workloads[i])
	}
	ref := func(kind string, m snapshot.ObjectMeta) snapshot.ResourceRef {
		return snapshot.ResourceRef{Kind: kind, Namespace: m.Namespace, Name: m.Name}
	}
	for i := range sp.Services {
		add(ref("Service", sp.Services[i].ObjectMeta), sp.Services[i])
	}
	for i := range sp.PodDisruptionBudgets {
		add(ref("PodDisruptionBudget", sp.PodDisruptionBudgets[i].ObjectMeta), sp.PodDisruptionBudgets[i])
	}
	for i := range sp.HorizontalPodAutoscalers {
		add(ref("HorizontalPodAutoscaler", sp.HorizontalPodAutoscalers[i].ObjectMeta), sp.HorizontalPodAutoscalers[i])
	}
	for i := range sp.ConfigMaps {
		add(ref("ConfigMap", sp.ConfigMaps[i].ObjectMeta), sp.ConfigMaps[i])
	}
	for i := range sp.Secrets {
		add(ref("Secret", sp.Secrets[i].ObjectMeta), sp.Secrets[i])
	}
	return out
}

// manifestPaths maps the snapshot's top-level fields to where they sit in a
// manifest, per kind. Fields not listed keep their snapshot name under
// spec. Identity fields (kind, name, namespace) are not compared.
func manifestPaths(kind string, obj map[string]any) map[string]string {
	podTemplate := "spec.template"
	if kind == snapshot.KindCronJob {
		podTemplate = "spec.jobTemplate.spec.template"
	}
	switch kind {
	case "Namespace":
		return map[string]string{"labels": "metadata.labels"}
	case "ConfigMap", "Secret":
		return map[string]string{"labels": "metadata.labels", "keys": "data", "type": "type"}
	case "Service":
		return map[string]string{"labels": "metadata.labels"}
	case "PodDisruptionBudget":
		return map[string]string{"labels": "metadata.labels", "apiVersion": "apiVersion", "selector": "spec.selector.matchLabels"}
	case "HorizontalPodAutoscaler":
		return map[string]string{"labels": "metadata.labels", "apiVersion": "apiVersion", "targetRef": "spec.scaleTargetRef"}
	}
	if _, ok := obj["template"]; ok { // a workload
		return map[string]string{
			"labels": "metadata.labels", "apiVersion": "apiVersion", "selector": "spec.selector.matchLabels",
			"template.labels": podTemplate + ".metadata.labels", "template.spec": podTemplate + ".spec",
		}
	}
	return map[string]string{"labels": "metadata.labels"}
}

// fields flattens an object to manifest field paths and normalized values.
// List items with a name are addressed as [name=...], other items by
// position, and map keys holding dots or slashes as [key].
func fields(kind string, obj map[string]any, norm func(string) string) map[string]string {
	out := map[string]string{}
	paths := manifestPaths(kind, obj)
	for k, v := range obj {
		switch k {
		case "kind", "name", "namespace":
			continue
		case "template":
			if t, ok := v.(map[string]any); ok {
				for tk, tv := range t {
					flatten(out, paths["template."+tk], tv, norm)
				}
			}
			continue
		case "keys": // ConfigMap and Secret key names: presence only
			if keys, ok := v.([]any); ok {
				for _, key := range keys {
					out[paths[k]+segment(fmt.Sprint(key))] = "(present)"
				}
			}
			continue
		}
		p, ok := paths[k]
		if !ok {
			p = "spec." + k
		}
		flatten(out, p, v, norm)
	}
	return out
}

func flatten(out map[string]string, path string, v any, norm func(string) string) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			out[path] = "{}"
		}
		for k, child := range x {
			flatten(out, path+segment(k), child, norm)
		}
	case []any:
		if len(x) == 0 {
			out[path] = "[]"
		}
		for i, item := range x {
			if m, ok := item.(map[string]any); ok {
				if name, ok := m["name"].(string); ok && name != "" {
					rest := make(map[string]any, len(m))
					for k, val := range m {
						if k != "name" {
							rest[k] = val
						}
					}
					key := path + "[name=" + norm(name) + "]"
					if len(rest) == 0 {
						out[key] = "(present)"
					}
					for k, child := range rest {
						flatten(out, key+segment(k), child, norm)
					}
					continue
				}
			}
			flatten(out, path+"["+strconv.Itoa(i)+"]", item, norm)
		}
	case string:
		out[path] = norm(x)
	case nil:
		out[path] = "null"
	default:
		raw, _ := json.Marshal(x)
		out[path] = string(raw)
	}
}

func segment(key string) string {
	if strings.ContainsAny(key, "./[]") {
		return "[" + key + "]"
	}
	return "." + key
}

func unionKeys(a, b map[string]string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range []map[string]string{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func valueOr(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}
	return unset
}
