package results

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A trial's transcript, verifier report and files tell what its agent did,
// what it ran into and why it missed.
func TestAnalyzeTrial(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("agent/trajectory.json", `{"steps":[
	  {"source":"user"},
	  {"source":"agent","tool_calls":[
	    {"tool_call_id":"1","function_name":"bash","arguments":{"command":"cd /app && axx run --compact"}},
	    {"tool_call_id":"2","function_name":"axx_scenarios_run","arguments":{}},
	    {"tool_call_id":"3","function_name":"skill","arguments":{"name":"axx-acceptance-tests"}},
	    {"tool_call_id":"4","function_name":"webfetch","arguments":{"url":"https://axx.nimbusxr.us"}},
	    {"tool_call_id":"5","function_name":"bash","arguments":{"command":"echo PathNotFoundException"}}],
	   "observation":{"results":[
	    {"source_call_id":"1","content":"FAIL ... No service set; register one with \"the {word} service with the following properties:\""},
	    {"source_call_id":"2","content":"{\"message\":\"Could not perform seed: insert into parcels.parcels\"}"},
	    {"source_call_id":"5","content":"PathNotFoundException"}]}}]}`)
	write("verifier/verify.json", `{"checks":[
	  {"name":"static rules","passed":true,"core":true},
	  {"name":"at least 3 passing scenarios","passed":false,"core":false,"detail":"1 passed"},
	  {"name":"catches mutant delete-not-removed","passed":false,"core":true,"detail":"1 scenario(s) passed"}],
	 "static":{"changes":[{"path":"features/a.feature","kind":"added"},{"path":"seeds/gone.yaml","kind":"deleted"}]}}`)
	write("artifacts/app/features/a.feature", "Feature: a\n  Scenario: b\n    Then c")
	in := analyzeTrial(dir, 0)
	w := in.work
	if w.ToolCalls != 5 || w.AxxCommands != 1 || w.MCPCalls != 1 || w.SkillsLoaded != 1 || w.DocsFetched != 1 || w.ModelRequests != 1 {
		t.Errorf("work = %+v", w)
	}
	if len(w.Lines) != 1 || w.Lines[0] != 3 {
		t.Errorf("lines = %v", w.Lines)
	}
	// Only axx's own output counts: the echo is not an axx command.
	if in.stumbles["used a service before registering it"] != 1 || in.stumbles["seed rejected by the database"] != 1 || in.stumbles["bare PathNotFoundException"] != 0 {
		t.Errorf("stumbles = %v", in.stumbles)
	}
	if m := in.miss; m == nil || m.Why != missMissedBug || !m.Core {
		t.Errorf("miss = %+v (core checks come first)", m)
	}
}

// The report compares the suites with axx and without on the tasks both ran.
func TestWithAndWithout(t *testing.T) {
	f := &File{
		Agent: "opencode", Model: "openrouter/openai/gpt-6-luna", Date: "2026-10-03",
		Conditions: []string{"none", "plain"},
		Tasks: []Task{
			{ID: "a", Results: map[string]*Result{
				"none": {Trials: 3, Passed: 2, CorePassed: 3, Reward: 2.0 / 3, Core: 1, Work: &Work{ToolCalls: 30, Lines: []int{10, 20, 30}},
					Misses:   []Miss{{Trial: "a__x1", Why: missScenarios, Core: false, Detail: "at least 3 passing scenarios: 1 passed"}},
					Stumbles: map[string]Stumble{"used a service before registering it": {Trials: 1, Times: 2}}},
				"plain": {Trials: 3, Passed: 2, CorePassed: 2, Reward: 2.0 / 3, Core: 2.0 / 3, Work: &Work{ToolCalls: 15, Lines: []int{100, 200, 300}},
					Misses: []Miss{{Trial: "a__y2", Why: missFailedCorrect, Core: true, Detail: "exit 1"}}},
			}},
			{ID: "b", Results: map[string]*Result{"none": {Trials: 3, Passed: 3, CorePassed: 3, Reward: 1, Core: 1}}},
		},
	}
	md := f.Markdown()
	for _, want := range []string{
		"## With axx and without",
		"**With axx, 3 of 3 suites passed every core check (100%)**, across none; **without axx, 2 of 3 (67%)**.",
		"Without axx: 1 failed against the correct service.",
		"median of 20 lines per suite with axx and 200 without",
		"| Failed against the correct service | 0 | 1 |",
		"## How the agents worked",
		"| Used a service before registering it | 1/6 · 2 |",
		"- `a` (plain, y2): **failed against the correct service**. exit 1",
		"- `a` (none, x1): **fewer scenarios than the task has criteria (axx-only check)**",
		"Suites that passed every core check but missed an axx-only one: none 1",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}
