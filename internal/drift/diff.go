package drift

import (
	"encoding/json"
	"errors"
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
	"resources":             {"**.resources", "**.resources.**"},
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
	if o.imageTags && containerImage.MatchString(path) && imageRepo(base) == imageRepo(target) {
		return true
	}
	return false
}

// containerImage matches the image field of a container or init container.
var containerImage = regexp.MustCompile(`\.(containers|initContainers)\[[^\]]*\]\.image$`)

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
func Diff(base, target *snapshot.Snapshot, o Options) ([]rules.Finding, error) {
	baseName, targetName := overlayName(base), overlayName(target)
	m := Match(base, target)
	out := m.Findings(baseName, targetName)
	bObj, err := objects(base)
	if err != nil {
		return nil, err
	}
	tObj, err := objects(target)
	if err != nil {
		return nil, err
	}
	bNorm, tNorm := newNormalizer(base, o.hostnames), newNormalizer(target, o.hostnames)
	for _, p := range m.Pairs {
		bf := bNorm.fields(p.Base.Kind, bObj[p.Base.String()])
		tf := tNorm.fields(p.Target.Kind, tObj[p.Target.String()])
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
	return out, nil
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

// normalizer rewrites values so that what kustomize or the overlay changed
// on purpose compares equal.
type normalizer struct {
	// renamed maps "Kind/name" of a renamed object to its source name, for
	// the fields that reference another object.
	renamed   map[string]string
	namespace string
	// host replaces the overlay name inside hostnames (the hostnames preset).
	host func(string) string
}

func newNormalizer(s *snapshot.Snapshot, hostnames bool) *normalizer {
	n := &normalizer{renamed: map[string]string{}, host: func(v string) string { return v }}
	o := s.Source.Overlay
	if o == nil {
		return n
	}
	for ref, orig := range o.Origins {
		if r, err := snapshot.ParseResourceRef(ref); err == nil {
			n.renamed[r.Kind+"/"+r.Name] = orig
		}
	}
	n.namespace = o.Namespace
	if hostnames && o.Name != "" {
		n.host = hostnameNormalizer(o.Name)
	}
	return n
}

// value normalizes a plain string value: the overlay namespace becomes a
// placeholder when it is the whole value, and the overlay name inside a
// hostname does too.
func (n *normalizer) value(v string) string {
	if n.namespace != "" && v == n.namespace {
		return "{namespace}"
	}
	return n.host(v)
}

var hostname = regexp.MustCompile(`[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+`)

// hostnameNormalizer replaces name with {overlay} where it is a whole part
// of a DNS label (split on dots and dashes) inside something shaped like a
// hostname. A bare "staging", or "staging" in a URL path or image
// repository, is a real value and is left alone.
func hostnameNormalizer(name string) func(string) string {
	token := regexp.MustCompile(`(^|[.-])` + regexp.QuoteMeta(name) + `($|[.-])`)
	return func(v string) string {
		return hostname.ReplaceAllStringFunc(v, func(h string) string {
			// Twice, so adjacent tokens (acme-acme) both change.
			for range 2 {
				h = token.ReplaceAllString(h, "${1}{overlay}${2}")
			}
			return h
		})
	}
}

// references are the fields that name another object, with the field that
// gives its kind. A reference to a renamed object goes back to the source
// name, so acme-webshop and globex-webshop compare equal.
var references = []struct {
	name *regexp.Regexp
	kind func(fields map[string]string, prefix string) string
}{
	{regexp.MustCompile(`^(spec\.scaleTargetRef)\.name$`), func(f map[string]string, p string) string { return f[p+".kind"] }},
	{regexp.MustCompile(`^(.*\.volumes\[name=[^\]]*\])\.source$`), func(f map[string]string, p string) string {
		switch f[p+".type"] {
		case "configMap":
			return "ConfigMap"
		case "secret":
			return "Secret"
		}
		return ""
	}},
}

// fields flattens an object and rewrites references to renamed objects.
func (n *normalizer) fields(kind string, obj map[string]any) map[string]string {
	out := fields(kind, obj, n.value)
	for path, v := range out {
		for _, r := range references {
			if m := r.name.FindStringSubmatch(path); m != nil {
				if orig, ok := n.renamed[r.kind(out, m[1])+"/"+v]; ok {
					out[path] = orig
				}
			}
		}
	}
	return out
}

// objects indexes the JSON form of every object by ref.
func objects(s *snapshot.Snapshot) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	var errs []error
	add := func(ref snapshot.ResourceRef, v any) {
		raw, err := json.Marshal(v)
		var m map[string]any
		if err == nil {
			err = json.Unmarshal(raw, &m)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("drift: %s: %w", ref, err))
			return
		}
		out[ref.String()] = m
	}
	walk(&s.Spec, add)
	return out, errors.Join(errs...)
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
	case "ConfigMap", "Secret":
		return map[string]string{"labels": "metadata.labels", "keys": "data", "type": "type"}
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
					key := path + "[name=" + name + "]"
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
