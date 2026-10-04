package results

import (
	"strings"
	"testing"
)

// The page has, per model, the comparison with and without axx and the
// scores by task, and links the run.
func TestPage(t *testing.T) {
	f := &File{
		Date: "2026-10-03", Agent: "opencode", Model: "openrouter/openai/gpt-6-luna", Axx: "0.1.12",
		Conditions: []string{"none", "plain"},
		Tasks: []Task{{ID: "rest-crud-happy-path", Results: map[string]*Result{
			"none":  {Trials: 3, Passed: 3, CorePassed: 3, CostUSD: 0.03},
			"plain": {Trials: 3, Passed: 2, CorePassed: 2, CostUSD: 0.03},
		}}},
		CoreScores: map[string]float64{"none": 100, "plain": 66.7},
	}
	page := Page([]*File{f}, map[string]string{f.Model: "https://example.com/run"})
	for _, want := range []string{
		"title: Agent evaluations", "## openai/gpt-6-luna", "axx 0.1.12", "([the run](https://example.com/run))",
		"### With axx and without", "**With axx, 3 of 3 suites passed every core check (100%)**", "### Scores by task",
		"| `rest-crud-happy-path` |", "| **Core score** | **100.0** | **66.7** |", "The run's report lists every failed suite",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "Every miss") {
		t.Error("the page points to a section it does not have")
	}
}
