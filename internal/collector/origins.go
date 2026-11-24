package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// affix is the name prefix and suffix kustomize adds to an object that
// comes from one kustomization: its own namePrefix and nameSuffix combined
// with every kustomization above it. An outer prefix goes in front of an
// inner one, and an outer suffix after an inner one.
type affix struct{ prefix, suffix string }

// kustTree is what renames objects in a kustomization and everything it
// includes: the affixes each included kustomization produces, and the names
// of the ConfigMap and Secret generators, whose output gets a hash suffix.
//
// shortcut: the affixes are not tied to the directory an object came from,
// because kustomize drops that record at the end of a build and the code
// that keeps it is internal. So an object without a prefix whose name
// starts like a sibling's prefix loses that part (TestOriginsSiblingPrefixLimit).
// That needs two branches of one tree with different prefixes and an
// unlucky name; the fix is reading kustomize's origin annotations.
type kustTree struct {
	affixes    []affix
	generators map[string]bool
}

// generatorHash is the content hash kustomize appends to generated
// ConfigMaps and Secrets after any name suffix: ten hex digits with 0, 1,
// 3, a and e swapped for g, h, k, m and t (kustomize's hasher.encode).
var generatorHash = regexp.MustCompile(`-[2456789bcdfghkmt]{10}$`)

// unprefixed are the kinds kustomize never gives a namePrefix or nameSuffix.
var unprefixed = map[string]bool{"Namespace": true, "CustomResourceDefinition": true}

// readKustTree walks the kustomizations reachable from dir through
// resources, bases and components. Remote and file entries are skipped:
// only local directories can carry a namePrefix or a generator.
func readKustTree(dir string) (*kustTree, error) {
	t := &kustTree{generators: map[string]bool{}}
	seen := map[affix]map[string]bool{}
	onPath := map[string]bool{}
	var walk func(dir string, outer affix) error
	walk = func(dir string, outer affix) error {
		kfile := findKustomization(dir)
		if kfile == "" {
			return nil
		}
		dir, err := filepath.Abs(dir)
		if err != nil {
			return err
		}
		if onPath[dir] {
			return fmt.Errorf("%s includes itself", dir)
		}
		onPath[dir] = true
		defer delete(onPath, dir)
		var k struct {
			NamePrefix         string   `json:"namePrefix"`
			NameSuffix         string   `json:"nameSuffix"`
			Resources          []string `json:"resources"`
			Bases              []string `json:"bases"`
			Components         []string `json:"components"`
			ConfigMapGenerator []struct {
				Name string `json:"name"`
			} `json:"configMapGenerator"`
			SecretGenerator []struct {
				Name string `json:"name"`
			} `json:"secretGenerator"`
		}
		data, err := os.ReadFile(filepath.Join(dir, kfile))
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(data, &k); err != nil {
			return fmt.Errorf("%s: %w", filepath.Join(dir, kfile), err)
		}
		a := affix{prefix: outer.prefix + k.NamePrefix, suffix: k.NameSuffix + outer.suffix}
		if seen[a] == nil {
			seen[a] = map[string]bool{}
			t.affixes = append(t.affixes, a)
		}
		if seen[a][dir] {
			return nil
		}
		seen[a][dir] = true
		for _, g := range k.ConfigMapGenerator {
			t.generators[g.Name] = true
		}
		for _, g := range k.SecretGenerator {
			t.generators[g.Name] = true
		}
		for _, entry := range append(append(k.Resources, k.Bases...), k.Components...) {
			child := filepath.Join(dir, entry)
			if info, err := os.Stat(child); err == nil && info.IsDir() {
				if err := walk(child, a); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(dir, affix{}); err != nil {
		return nil, err
	}
	// Longest affix first, so the most specific explanation of a name wins.
	sort.SliceStable(t.affixes, func(i, j int) bool {
		return len(t.affixes[i].prefix)+len(t.affixes[i].suffix) > len(t.affixes[j].prefix)+len(t.affixes[j].suffix)
	})
	return t, nil
}

// original returns the name an object had in its source file.
func (t *kustTree) original(kind, name string) string {
	if unprefixed[kind] {
		return name
	}
	if kind == "ConfigMap" || kind == "Secret" {
		if loc := generatorHash.FindStringIndex(name); loc != nil {
			if orig, ok := t.strip(name[:loc[0]]); ok && t.generators[orig] {
				return orig
			}
		}
	}
	if orig, ok := t.strip(name); ok {
		return orig
	}
	return name
}

func (t *kustTree) strip(name string) (string, bool) {
	for _, a := range t.affixes {
		if len(name) > len(a.prefix)+len(a.suffix) && strings.HasPrefix(name, a.prefix) && strings.HasSuffix(name, a.suffix) {
			return name[len(a.prefix) : len(name)-len(a.suffix)], true
		}
	}
	return name, false
}

// origins maps every renamed object to its source name.
func origins(dir string, refs []string) (map[string]string, error) {
	t, err := readKustTree(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range refs {
		ref, err := snapshot.ParseResourceRef(r)
		if err != nil {
			return nil, err
		}
		if orig := t.original(ref.Kind, ref.Name); orig != ref.Name {
			out[r] = orig
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}
