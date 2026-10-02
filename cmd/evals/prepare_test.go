package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestPrepareDatasetOnlyTheNamedTasks: a run with --tasks prepares those
// tasks and no others, whichever sort after them.
func TestPrepareDatasetOnlyTheNamedTasks(t *testing.T) {
	root := filepath.Join("..", "..")
	out := t.TempDir()
	included, _, err := prepareDataset(root, Condition{Name: "none"}, out, []string{"fix-broken-feature", "rest-crud-happy-path"}, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"fix-broken-feature", "rest-crud-happy-path"}; !slices.Equal(included, want) {
		t.Fatalf("included %v, want %v", included, want)
	}
	entries, _ := os.ReadDir(out)
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if !slices.Equal(dirs, included) {
		t.Errorf("dataset holds %v", dirs)
	}
	if _, _, err := prepareDataset(root, Condition{Name: "none"}, t.TempDir(), []string{"no-such-task"}, nil, true); err == nil {
		t.Error("an unknown task was not refused")
	}
}
