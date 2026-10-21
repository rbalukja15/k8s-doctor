package checks

import (
	"strings"

	"github.com/rbalukja15/k8s-doctor/internal/rules"
	"github.com/rbalukja15/k8s-doctor/internal/snapshot"
)

// imageTagMutable flags images referenced by "latest" or by no tag at all,
// unless they are pinned by digest.
type imageTagMutable struct{ meta }

func newImageTagMutable() rules.Rule {
	return imageTagMutable{meta{
		id:       "image-tag-mutable",
		category: CategoryImages,
		severity: rules.SeverityMedium,
		modes:    rules.AllModes,
		description: "The image uses the \"latest\" tag or no tag, which means latest. The running version then " +
			"depends on when each node pulled it, rollbacks cannot return to a known version, and a manifest " +
			"no longer describes what is deployed. Pin a version tag or a digest.",
	}}
}

func (imageTagMutable) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, nil, func(w *snapshot.Workload, c *snapshot.Container) {
		ref := parseImage(c.Image)
		if !ref.mutable() {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".image",
			Message:   "container " + c.Name + " uses mutable image " + c.Image,
			Evidence:  map[string]string{"container": c.Name, "image": c.Image, "tag": ref.displayTag()},
		})
	})
	return out
}

// imagePullPolicyStale flags a mutable image combined with an explicit
// IfNotPresent or Never pull policy: nodes keep whatever copy they pulled
// first, so pods of the same workload can run different code.
type imagePullPolicyStale struct{ meta }

func newImagePullPolicyStale() rules.Rule {
	return imagePullPolicyStale{meta{
		id:       "image-pull-policy-stale",
		category: CategoryImages,
		severity: rules.SeverityHigh,
		modes:    rules.AllModes,
		description: "The image tag is mutable and imagePullPolicy is IfNotPresent or Never, so each node keeps " +
			"the copy it pulled first. Pods of the same workload can silently run different code, and a new " +
			"push under the same tag never reaches nodes that already have it.",
	}}
}

func (imagePullPolicyStale) Evaluate(s *snapshot.Snapshot) []rules.Finding {
	var out []rules.Finding
	eachContainer(s, nil, func(w *snapshot.Workload, c *snapshot.Container) {
		if c.ImagePullPolicy != "IfNotPresent" && c.ImagePullPolicy != "Never" {
			return
		}
		ref := parseImage(c.Image)
		if !ref.mutable() {
			return
		}
		out = append(out, rules.Finding{
			Resource:  w.Ref(),
			FieldPath: rules.ContainerPath(w, c) + ".imagePullPolicy",
			Message:   "container " + c.Name + " pulls mutable image " + c.Image + " with imagePullPolicy " + c.ImagePullPolicy,
			Evidence: map[string]string{
				"container":       c.Name,
				"image":           c.Image,
				"imagePullPolicy": c.ImagePullPolicy,
			},
		})
	})
	return out
}

type imageRef struct {
	tag    string
	digest string
}

// parseImage splits an image reference into tag and digest. A colon only
// starts a tag when it comes after the last slash, so a registry port
// ("registry:5000/app") is not mistaken for a tag.
func parseImage(image string) imageRef {
	var ref imageRef
	name := image
	if i := strings.Index(name, "@"); i >= 0 {
		ref.digest = name[i+1:]
		name = name[:i]
	}
	if i := strings.LastIndex(name, ":"); i > strings.LastIndex(name, "/") {
		ref.tag = name[i+1:]
	}
	return ref
}

func (r imageRef) mutable() bool {
	return r.digest == "" && (r.tag == "" || r.tag == "latest")
}

func (r imageRef) displayTag() string {
	if r.tag == "" {
		return "(none, defaults to latest)"
	}
	return r.tag
}
