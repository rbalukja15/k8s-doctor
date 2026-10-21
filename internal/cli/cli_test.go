package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rbalukja15/k8s-doctor/internal/cli"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = cli.Run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestUsage(t *testing.T) {
	code, out, _ := run()
	if code != cli.ExitOK || !strings.Contains(out, "Usage:") {
		t.Fatalf("no-arg run: code %d, output %q", code, out)
	}
	code, _, errOut := run("frobnicate")
	if code != cli.ExitUsage || !strings.Contains(errOut, "unknown command") {
		t.Fatalf("unknown command: code %d, stderr %q", code, errOut)
	}
}

func TestRules(t *testing.T) {
	code, out, _ := run("rules")
	if code != cli.ExitOK || !strings.Contains(out, "probe-readiness-missing") {
		t.Fatalf("rules: code %d, output %q", code, out)
	}
}

func TestCheck(t *testing.T) {
	code, out, errOut := run("check", "--snapshot", "../../fixtures/image-tag-mutable/positive/snapshot.json")
	if code != cli.ExitOK {
		t.Fatalf("check: code %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "image-tag-mutable") || !strings.Contains(out, "Deployment/shop/web") {
		t.Errorf("check output is missing the expected finding:\n%s", out)
	}

	code, _, _ = run("check")
	if code != cli.ExitUsage {
		t.Errorf("check without --snapshot: code %d, want %d", code, cli.ExitUsage)
	}
	code, _, _ = run("check", "--snapshot", "does-not-exist.json")
	if code != cli.ExitError {
		t.Errorf("check with a missing file: code %d, want %d", code, cli.ExitError)
	}
}
