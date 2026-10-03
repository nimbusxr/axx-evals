package main

import (
	"fmt"
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

// TestShardTasksCoverEveryTaskOnce: the parts of a sharded run hold every
// task, each in one part, as CI splits the plumbing run across jobs.
func TestShardTasksCoverEveryTaskOnce(t *testing.T) {
	root := filepath.Join("..", "..")
	all, err := shardTasks(root, "1/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for i := 1; i <= 4; i++ {
		part, err := shardTasks(root, fmt.Sprintf("%d/4", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, part...)
	}
	slices.Sort(got)
	if !slices.Equal(got, all) {
		t.Errorf("the shards hold %v, want %v", got, all)
	}
	if part, err := shardTasks(root, "1/4", []string{all[0], all[1]}); err != nil || !slices.Equal(part, all[:1]) {
		t.Errorf("--tasks with --shard: %v, %v", part, err)
	}
	for _, bad := range []string{"0/4", "5/4", "4", "a/b"} {
		if _, err := shardTasks(root, bad, nil); err == nil {
			t.Errorf("--shard %s was not refused", bad)
		}
	}
}
