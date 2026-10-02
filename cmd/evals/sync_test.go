package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedFilesUpToDate fails when a task's generated files (Dockerfiles,
// compose files, test.sh, the initial manifest) are stale: run
// `go run ./cmd/evals sync`.
func TestGeneratedFilesUpToDate(t *testing.T) {
	stale, err := syncTasks(filepath.Join("..", ".."), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) > 0 {
		t.Fatalf("stale generated files (run `go run ./cmd/evals sync`):\n  %s", strings.Join(stale, "\n  "))
	}
}

// TestWorkspaceCopies keeps the starting project's copies of the service's
// contract and stubs identical to the originals in app/.
func TestWorkspaceCopies(t *testing.T) {
	root := filepath.Join("..", "..")
	pairs := map[string]string{
		"workspace/openapi.yaml": "app/openapi.yaml",
		"tasks/init-first-feature/environment/workspace/openapi.yaml":          "app/openapi.yaml",
		"workspace/infra/address-service/mappings/postcode-deliverable.json":   "app/wiremock/mappings/postcode-deliverable.json",
		"workspace/infra/address-service/mappings/postcode-undeliverable.json": "app/wiremock/mappings/postcode-undeliverable.json",
		"tasks/init-first-feature/environment/workspace/docs/registration.md":  "workspace/docs/registration.md",
	}
	for cp, orig := range pairs {
		a, err := os.ReadFile(filepath.Join(root, cp))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(root, orig))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from %s: copy it again", cp, orig)
		}
	}
}

func TestApplyCondition(t *testing.T) {
	p := filepath.Join(t.TempDir(), "Dockerfile")
	if err := os.WriteFile(p, []byte("FROM x\n"+conditionMarker+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := applyCondition(p, Condition{Name: "both", AgentsMD: true, Skills: true, MCP: true}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "RUN project-setup finish --agents-md --skills\n") {
		t.Fatalf("Dockerfile:\n%s", b)
	}
}

// TestPlainExclusions keeps project-setup plain, which builds the starting
// project without axx in the container, removing what plainManifest leaves out.
func TestPlainExclusions(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "images", "base", "bin", "project-setup"))
	if err != nil {
		t.Fatal(err)
	}
	var rm []string
	for _, p := range plainExcluded {
		rm = append(rm, `"$app/`+p+`"`)
	}
	if want := "rm -rf " + strings.Join(rm, " "); !strings.Contains(string(b), want) {
		t.Errorf("project-setup plain does not run %q", want)
	}
}

// The rate-limit proxy ties a request to its trial by the agent container's
// name, which Harbor derives from the trial's (in lower case).
func TestHarborContainerNames(t *testing.T) {
	for name, want := range map[string]string{
		"/rest-crud-happy-path__kzffvox__env-main-1":  "rest-crud-happy-path__kzffvox",
		"openapi-reject-invalid__edgjk4l__env-main-1": "openapi-reject-invalid__edgjk4l",
		"/rest-crud-happy-path__kzffvox__env-mongo-1": "",
	} {
		got := ""
		if m := harborContainer.FindStringSubmatch(name); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}
