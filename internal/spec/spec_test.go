package spec

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nimbusxr/axx-evals/internal/mutant"
)

// TestTasks loads every task of the repository: task.toml and verify.toml
// must parse, name known mutants and variants, and every mutant and variant
// must be used by at least one task. A version without axx holds its tests to
// the same mutants and variants as the axx version, so the two compare.
func TestTasks(t *testing.T) {
	tasks, err := LoadTasks(filepath.Join("..", "..", "tasks"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) < 9 {
		t.Fatalf("%d tasks, want at least 9", len(tasks))
	}
	targeted, varied := map[string]bool{}, map[string]bool{}
	for _, task := range tasks {
		v, err := LoadVerify(filepath.Join(task.Dir, "tests", "verify.toml"))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range v.Mutants {
			targeted[m] = true
		}
		for _, n := range v.Variants {
			varied[n] = true
		}
		if task.PlainDir != "" {
			pv, err := LoadVerify(filepath.Join(task.PlainDir, "tests", "verify.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(pv.Mutants, v.Mutants) || !slices.Equal(pv.Variants, v.Variants) {
				t.Errorf("%s: the version without axx has mutants %v and variants %v, the axx version %v and %v",
					task.ID(), pv.Mutants, pv.Variants, v.Mutants, v.Variants)
			}
		}
		for _, f := range []string{"instruction.md", "solution/solve.sh"} {
			if _, err := os.Stat(filepath.Join(task.Dir, f)); err != nil {
				t.Errorf("%s: %v", task.ID(), err)
			}
		}
		if task.Meta.Title == "" || task.Meta.Category == "" {
			t.Errorf("%s: metadata title and category are required", task.ID())
		}
		if !strings.HasPrefix(task.Name, "axx-evals/") || !strings.HasSuffix(task.Name, "/"+task.ID()) {
			t.Errorf("%s: task name %q must be axx-evals/<directory>", task.ID(), task.Name)
		}
	}
	for _, m := range mutant.All {
		if !targeted[m.Name] {
			t.Errorf("mutant %s is not targeted by any task", m.Name)
		}
	}
	for _, n := range mutant.Variants {
		if !varied[n.Name] {
			t.Errorf("variant %s is not used by any task", n.Name)
		}
	}
}

func TestLoadVerifyRejects(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"unknown-mutant": "allowed = [\"features/**\"]\nmin_scenarios = 1\nmutants = [\"nope\"]\n",
		"unknown-field":  "allowed = [\"features/**\"]\nmin_scenarios = 1\nmutants = [\"no-weight-limit\"]\nbogus = 1\n",
		"no-allowed":     "min_scenarios = 1\nmutants = [\"no-weight-limit\"]\n",
	} {
		p := filepath.Join(dir, name+".toml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadVerify(p); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestAllowsFixtureFactoryFiles(t *testing.T) {
	v := &Verify{Allowed: []string{"features/**", "seeds/**"}}
	for p, want := range map[string]bool{
		"seeds/manifest.factory.yaml":     true,
		"seeds/lines/late.fixture.yaml":   true,
		"axx-fixtures.manifest.yaml":      true,
		"axx-fixtures.pairings.yaml":      true,
		"axx-lint.generated.yaml":         true,
		"seeds/manifest-lines.yaml":       true,
		"features/import.feature":         true,
		"app/main.go":                     false,
		"fixtures/generated/lines.yaml":   false, // a generated file outside the allowed paths
		"docs/manifest.factory.yaml.orig": false,
	} {
		if got := v.AllowsPath(p); got != want {
			t.Errorf("AllowsPath(%q) = %v, want %v", p, got, want)
		}
	}
}

// The init task forbids only changing README.md, openapi.yaml and the docs,
// as its instruction says: request payloads may live wherever the agent
// keeps them.
func TestInitTaskAllowsWhatItsInstructionAllows(t *testing.T) {
	v, err := LoadVerify(filepath.Join("..", "..", "tasks", "init-first-feature", "tests", "verify.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{
		"axx.yaml":                       true,
		"features/register.feature":      true,
		"resources/parcel-register.json": true,
		"README.md":                      false,
		"openapi.yaml":                   false,
		"docs/registration.md":           false,
	} {
		if got := v.AllowsPath(p); got != want {
			t.Errorf("AllowsPath(%q) = %v, want %v", p, got, want)
		}
	}
}
