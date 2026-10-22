// Package version holds the build version, set at link time:
//
//	go build -ldflags "-X github.com/rbalukja15/k8s-doctor/internal/version.Version=v1.0.0"
package version

// Version is the release tag, or "dev" for local builds.
var Version = "dev"
