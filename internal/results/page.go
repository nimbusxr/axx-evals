package results

import (
	"fmt"
	"sort"
	"strings"
)

// Page renders the results page of axx's docs site from each model's
// result file: what the evals measure, then per model the comparison with
// and without axx and the scores by task. links maps a model to its run.
func Page(files []*File, links map[string]string) string {
	var b strings.Builder
	b.WriteString(`---
title: Agent evaluations
description: How coding agents do with axx and without it, on the same tasks, held to the same checks, with bugs planted in the service.
---

<!-- Written by ` + "`go run ./cmd/evals page`" + ` in nimbusxr/axx-evals from the runs' result files; do not edit by hand. -->

Coding agents write acceptance tests for a parcels service (REST and OpenAPI, PostgreSQL, MongoDB, Kafka, a downstream service mocked with WireMock), from acceptance criteria, as a developer would ask them to. Each task runs under five conditions: with axx and none of its aids (` + "`none`" + `), with its agent skills (` + "`skills`" + `), its MCP server (` + "`mcp`" + `), both (` + "`both`" + `), and without axx at all, tests of any kind run by a script (` + "`plain`" + `).

Every suite is held to the same **core checks**: it passes against the correct service, twice and against variants that differ only where the contract allows, and it fails against every bug planted in the service. A suite that passes them all would catch those bugs in a real project. Everything is public in [nimbusxr/axx-evals](https://github.com/nimbusxr/axx-evals): the tasks, the planted bugs, the verifier, and every run's transcripts.
`)
	sorted := append([]*File(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Model < sorted[j].Model })
	for _, f := range sorted {
		model := strings.TrimPrefix(f.Model, "openrouter/")
		fmt.Fprintf(&b, "\n## %s\n\n", model)
		fmt.Fprintf(&b, "%s, axx %s, %s through OpenRouter", f.Date, orDash(f.Axx), f.Agent)
		if link := links[f.Model]; link != "" {
			fmt.Fprintf(&b, " ([the run](%s))", link)
		}
		b.WriteString(".\n\n")
		var w strings.Builder
		f.withAndWithout(&w)
		section := strings.Replace(w.String(), "## With axx and without\n\n", "### With axx and without\n\n", 1)
		section = strings.Replace(section, `Every failed suite is listed under "Every miss" below.`, "The run's report lists every failed suite and why.", 1)
		b.WriteString(section)
		b.WriteString("### Scores by task\n\nThe share of each task's trials that passed every check; the core score holds every condition to the same checks, and the score adds what only an axx suite has (`axx validate`, the features' readability, `axx lint` where the task asks).\n\n")
		b.WriteString("| Task |")
		for _, c := range f.Conditions {
			fmt.Fprintf(&b, " %s |", c)
		}
		b.WriteString("\n| --- |")
		for range f.Conditions {
			b.WriteString(" --- |")
		}
		b.WriteString("\n")
		tasks := append([]Task(nil), f.Tasks...)
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		for _, t := range tasks {
			fmt.Fprintf(&b, "| `%s` |", t.ID)
			for _, c := range f.Conditions {
				fmt.Fprintf(&b, " %s |", cell(t.Results[c]))
			}
			b.WriteString("\n")
		}
		for _, row := range []struct {
			name   string
			scores map[string]float64
		}{{"Core score", f.CoreScores}, {"Score", f.Scores}} {
			if row.scores == nil {
				continue
			}
			fmt.Fprintf(&b, "| **%s** |", row.name)
			for _, c := range f.Conditions {
				if s, ok := row.scores[c]; ok {
					fmt.Fprintf(&b, " **%.1f** |", s)
				} else {
					b.WriteString(" - |")
				}
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
