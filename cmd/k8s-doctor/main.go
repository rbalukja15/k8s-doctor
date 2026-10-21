// Command k8s-doctor diagnoses Kubernetes clusters and manifests without
// ever changing them.
package main

import (
	"os"

	"github.com/rbalukja15/k8s-doctor/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
