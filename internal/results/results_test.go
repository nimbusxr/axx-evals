package results

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func file(agent string, rewards map[string]map[string]float64) *File {
	f := &File{SchemaVersion: SchemaVersion, Kind: Kind, Agent: agent, Conditions: []string{"none", "skills"}}
	for id, byCond := range rewards {
		t := Task{ID: id, Results: map[string]*Result{}}
		for c, r := range byCond {
			passed := 0
			if r >= 1 {
				passed = 1
			}
			t.Results[c] = &Result{Reward: r, Trials: 1, Passed: passed}
		}
		f.Tasks = append(f.Tasks, t)
	}
	f.Scores = f.Score(nil)
	return f
}

func TestCompare(t *testing.T) {
	base := file("claude-code", map[string]map[string]float64{
		"a": {"none": 1, "skills": 1}, "b": {"none": 1, "skills": 1}, "c": {"none": 0, "skills": 1}, "d": {"none": 0, "skills": 1},
	})
	same := file("claude-code", map[string]map[string]float64{
		"a": {"none": 1, "skills": 1}, "b": {"none": 1, "skills": 1}, "c": {"none": 0, "skills": 1}, "d": {"none": 1, "skills": 1},
		"new": {"none": 0, "skills": 0}, // not in the baseline: ignored
	})
	c, err := Compare(base, same, 10)
	if err != nil {
		t.Fatal(err)
	}
	if c.Regressed {
		t.Fatalf("no regression expected: %s", c)
	}
	worse := file("claude-code", map[string]map[string]float64{
		"a": {"none": 1, "skills": 1}, "b": {"none": 1, "skills": 0}, "c": {"none": 0, "skills": 1}, "d": {"none": 0, "skills": 1},
	})
	c, err = Compare(base, worse, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Regressed {
		t.Fatalf("skills dropped 25 points; want a regression: %s", c)
	}
	if !strings.Contains(c.String(), "REGRESSION") || len(c.TaskChanges) != 1 {
		t.Fatalf("report: %s", c)
	}
	if _, err := Compare(base, file("codex", nil), 10); err == nil {
		t.Fatal("comparing different agents must fail")
	}
}

func TestMarkdownAndRoundTrip(t *testing.T) {
	f := file("claude-code", map[string]map[string]float64{"a": {"none": 1, "skills": 0}})
	f.Skipped = []Skipped{{ID: "k", Reason: "requires the kafka pack(s), not in this axx build"}}
	md := f.Markdown()
	for _, want := range []string{"| a | - | pass | fail |", "**100.0** | **0.0**", "`k`"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	p := filepath.Join(t.TempDir(), "r.json")
	if err := f.Write(p); err != nil {
		t.Fatal(err)
	}
	back, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if back.Scores["none"] != 100 || back.Scores["skills"] != 0 {
		t.Fatalf("scores = %v", back.Scores)
	}
}

func TestFromHarborJob(t *testing.T) {
	job := t.TempDir()
	trial := func(name, body, verify string) {
		d := filepath.Join(job, name)
		if err := os.MkdirAll(filepath.Join(d, "verifier"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "result.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if verify != "" {
			if err := os.WriteFile(filepath.Join(d, "verifier", "verify.json"), []byte(verify), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	trial("a__1", `{"task_name":"axx-evals/a","task_id":{"path":"/x/a"},"verifier_result":{"rewards":{"reward":1}},"exception_info":null}`,
		`{"mutantsTotal":2,"mutants":{"m1":{"killed":true},"m2":{"killed":true}}}`)
	trial("a__2", `{"task_name":"axx-evals/a","task_id":{"path":"/x/a"},"verifier_result":{"rewards":{"reward":0}},"exception_info":null}`,
		`{"mutantsTotal":2,"mutants":{"m1":{"killed":true},"m2":{"killed":false}}}`)
	trial("b__1", `{"task_name":"axx-evals/b","task_id":{"path":"b"},"verifier_result":null,"exception_info":{"exception_type":"AgentTimeoutError"},"agent_result":{"cost_usd":0.5,"n_input_tokens":10,"n_output_tokens":2}}`, "")
	// Cut off by the provider's rate limit or the infrastructure: not scored,
	// though what it spent counts.
	trial("a__3", `{"task_name":"axx-evals/a","task_id":{"path":"/x/a"},"verifier_result":{"rewards":{"reward":1}},"exception_info":{"exception_type":"ApiRateLimitError"},"agent_result":{"cost_usd":0.25}}`, "")
	trial("c__1", `{"task_name":"axx-evals/c","task_id":{"path":"c"},"verifier_result":null,"exception_info":{"exception_type":"RuntimeError"}}`, "")
	res, err := FromHarborJob(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c := res["c"]; c == nil || c.Trials != 0 || c.Unscored["RuntimeError"] != 1 || cell(c) != "not scored (1)" {
		t.Fatalf("c = %+v (%s)", c, cell(c))
	}
	if a := res["a"]; a.Unscored["ApiRateLimitError"] != 1 || a.CostUSD != 0.25 || cell(a) != "1/2 (1 not scored)" {
		t.Fatalf("a = %+v (%s)", a, cell(a))
	}
	a, b := res["a"], res["b"]
	if a == nil || a.Trials != 2 || a.Passed != 1 || a.Reward != 0.5 || a.MutantsCaught != 3 {
		t.Fatalf("a = %+v", a)
	}
	if b == nil || b.Errors != 1 || b.Reward != 0 || b.CostUSD != 0.5 {
		t.Fatalf("b = %+v", b)
	}
}

func TestUnexpected(t *testing.T) {
	f := &File{
		Conditions: []string{"none", "both"},
		Tasks: []Task{
			{ID: "a", Results: map[string]*Result{"none": {Reward: 1, Core: 1, Trials: 1}, "both": {Reward: 1, Core: 1, Trials: 1}}},
			{ID: "b", Results: map[string]*Result{"none": {Reward: 0, Trials: 1}, "both": {Reward: 1, Trials: 1, Errors: 1}}},
			{ID: "c", Results: map[string]*Result{"none": {Reward: 1, Core: 1, Trials: 1}}},
			{ID: "f", Results: map[string]*Result{"none": {Reward: 1, Core: 1, Trials: 1}}},
			{ID: "g", Results: map[string]*Result{"none": {Reward: 1, Core: 1, Trials: 1, Unscored: map[string]int{"ApiRateLimitError": 2}}, "both": {Reward: 1, Core: 1, Trials: 1}}},
		},
		Skipped: []Skipped{
			{ID: "d", Reason: "requires the x pack(s)"},
			{ID: "e", Condition: "both", Reason: "no version without axx", ByDesign: true},
			{ID: "f", Condition: "both", Reason: "no version without axx", ByDesign: true},
		},
	}
	got := f.Unexpected(1)
	want := []string{
		"d: skipped (requires the x pack(s))",
		"b (none): reward 0, want 1",
		"b (both): 1 trial(s) ended in an exception",
		"c (both): no trial ran",
		"g (none): 2 trial(s) not scored",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("Unexpected(1) =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnscoredMakesTheRunIncomplete(t *testing.T) {
	f := &File{
		Agent: "opencode", Date: "2026-10-02", Axx: "0.1.8", Harbor: "0.23.0",
		Conditions: []string{"none"},
		Tasks: []Task{{ID: "a", Results: map[string]*Result{"none": {Reward: 1, Core: 1, Trials: 2, Passed: 2,
			Unscored: map[string]int{"ApiRateLimitError": 1, "RuntimeError": 1}}}}},
	}
	got := f.Unscored()
	want := "a (none): 2 trial(s) not scored: ApiRateLimitError ×1, RuntimeError ×1"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("Unscored() = %q, want %q", got, want)
	}
	if md := f.Markdown(); !strings.Contains(md, "**Incomplete run:**") || !strings.Contains(md, "2/2 (2 not scored)") {
		t.Errorf("markdown:\n%s", md)
	}
}

// The waits on the rate limit are not the agent's: its working time leaves
// them out, the budget holds it to that working time, and a trial Harbor
// timed out only because of the waits is not scored.
func TestRateLimitWaitsAreLeftOut(t *testing.T) {
	job := t.TempDir()
	trial := func(name, body string) {
		d := filepath.Join(job, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "result.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(secs int) string {
		start := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
		return fmt.Sprintf(`"agent_execution":{"started_at":%q,"finished_at":%q}`, start.Format(time.RFC3339Nano), start.Add(time.Duration(secs)*time.Second).Format(time.RFC3339Nano))
	}
	// Ran 100 s, 60 of them waiting: 40 s of work.
	trial("a__1", `{"task_name":"axx-evals/a","task_id":{"path":"a"},"verifier_result":{"rewards":{"reward":1,"core":1}},`+run(100)+`}`)
	// Worked 2000 s against a budget of 1800: a timeout, though the verifier passed it.
	trial("a__2", `{"task_name":"axx-evals/a","task_id":{"path":"a"},"verifier_result":{"rewards":{"reward":1,"core":1}},`+run(2000)+`}`)
	// Timed out by Harbor after 7200 s, 6000 of them waiting: within its budget, so not scored.
	trial("a__3", `{"task_name":"axx-evals/a","task_id":{"path":"a"},"verifier_result":{"rewards":{"reward":0,"core":0}},"exception_info":{"exception_type":"AgentTimeoutError"},`+run(7200)+`}`)
	rl := `{"trials":{"a__1":{"waitSeconds":60,"refused":2,"requests":20},"a__3":{"waitSeconds":6000,"refused":90,"requests":40}}}`
	if err := os.WriteFile(filepath.Join(job, RateLimitFile), []byte(rl), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := FromHarborJob(job, map[string]float64{"a": 1800})
	if err != nil {
		t.Fatal(err)
	}
	a := res["a"]
	if a.Trials != 2 || a.Passed != 1 || a.Errors != 1 || a.Core != 0.5 {
		t.Fatalf("scored: %+v", a)
	}
	if a.Unscored[waitedOut] != 1 {
		t.Fatalf("unscored: %v", a.Unscored)
	}
	if a.AgentSeconds != 40+2000 || a.RateLimitWaitSeconds != 6060 || a.RateLimitRefused != 92 {
		t.Fatalf("times: %+v", a)
	}
}

// A run shows any model its agents called besides the one under test.
func TestOtherModels(t *testing.T) {
	f := &File{
		Agent: "opencode", Model: "openrouter/openai/gpt-6-luna", Date: "2026-10-02", Axx: "0.1.8", Harbor: "0.23.0",
		Conditions: []string{"none", "plain"},
		Tasks: []Task{{ID: "a", Results: map[string]*Result{
			"none":  {Trials: 1, Passed: 1, Reward: 1, Core: 1, Models: map[string]int{"openai/gpt-6-luna": 22, "google/gemini-3.8-flash": 1}},
			"plain": {Trials: 1, Passed: 1, Reward: 1, Core: 1, Models: map[string]int{"openai/gpt-6-luna": 23}},
		}}},
	}
	got := f.OtherModels()
	if len(got) != 1 || got[0] != "a (none): google/gemini-3.8-flash ×1" {
		t.Fatalf("OtherModels() = %q", got)
	}
	if md := f.Markdown(); !strings.Contains(md, "**Other models called:**") {
		t.Errorf("markdown:\n%s", md)
	}
}
