package results

import (
	"fmt"
	"sort"
	"strings"
)

// The report's sections beyond the scores (File.Markdown): axx against no
// axx, how the agents worked, where they stumbled, and every miss. Each is
// computed from the trials, so a run's report needs no reading of
// transcripts to understand.

// plainCondition is the condition without axx.
const plainCondition = "plain"

// condStats sums a condition's results over some tasks.
type condStats struct {
	trials, corePassed, unscored int
	coreMisses                   map[string]int // by Miss.Why
	work                         Work
	agentSeconds, costUSD        float64
	inputTokens                  int64
}

func (f *File) stats(c string, tasks []Task) condStats {
	s := condStats{coreMisses: map[string]int{}}
	for _, t := range tasks {
		r := t.Results[c]
		if r == nil {
			continue
		}
		s.trials += r.Trials
		s.corePassed += r.CorePassed
		s.unscored += r.UnscoredCount()
		s.agentSeconds += r.AgentSeconds
		s.costUSD += r.CostUSD
		s.inputTokens += r.InputTokens
		if r.Work != nil {
			s.work.add(*r.Work)
		}
		for _, m := range r.Misses {
			if m.Core {
				s.coreMisses[m.Why]++
			}
		}
	}
	return s
}

// perTrial divides by the scored trials; ok is false without any.
func (s condStats) perTrial(v float64) (float64, bool) {
	if s.trials == 0 {
		return 0, false
	}
	return v / float64(s.trials), true
}

func median(xs []int) (float64, bool) {
	if len(xs) == 0 {
		return 0, false
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	if n := len(s); n%2 == 1 {
		return float64(s[n/2]), true
	}
	return float64(s[len(s)/2-1]+s[len(s)/2]) / 2, true
}

// row writes one table row of per-condition values.
func row(b *strings.Builder, name string, conds []string, value func(c string) string) {
	fmt.Fprintf(b, "| %s |", name)
	for _, c := range conds {
		fmt.Fprintf(b, " %s |", value(c))
	}
	b.WriteString("\n")
}

func header(b *strings.Builder, first string, conds []string) {
	fmt.Fprintf(b, "| %s |", first)
	for _, c := range conds {
		if c == plainCondition {
			c = "plain (no axx)"
		}
		fmt.Fprintf(b, " %s |", c)
	}
	b.WriteString("\n| --- |")
	for range conds {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
}

// sentence starts a row name with a capital, leaving axx as it is written.
func sentence(s string) string {
	if s == "" || strings.HasPrefix(s, "axx") {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func num(v float64, ok bool, format string) string {
	if !ok {
		return "-"
	}
	return fmt.Sprintf(format, v)
}

// sharedTasks are the tasks the plain condition ran: the ones to compare axx
// and no axx on.
func (f *File) sharedTasks() []Task {
	var out []Task
	for _, t := range f.Tasks {
		if r := t.Results[plainCondition]; r != nil && r.Trials > 0 {
			out = append(out, t)
		}
	}
	return out
}

func pct(a, b int) string {
	if b == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", float64(a)*100/float64(b))
}

// missOrder is the order the core misses are told in.
var missOrder = []string{missFailedCorrect, missFailedVariant, missMissedBug, missStatic, missNoChange, missOverBudget, missTimedOut, missCrashed, missOther}

// withAndWithout compares the conditions with axx and without on the tasks
// they all ran: did the suites hold, and what did they cost.
func (f *File) withAndWithout(b *strings.Builder) {
	shared := f.sharedTasks()
	hasPlain := false
	for _, c := range f.Conditions {
		hasPlain = hasPlain || c == plainCondition
	}
	if !hasPlain || len(shared) == 0 {
		return
	}
	st := map[string]condStats{}
	for _, c := range f.Conditions {
		st[c] = f.stats(c, shared)
	}
	b.WriteString("## With axx and without\n\n")
	ids := make([]string, len(shared))
	for i, t := range shared {
		ids[i] = "`" + t.ID + "`"
	}
	fmt.Fprintf(b, "The %d tasks that have a version without axx (%s), each condition's suites held to the same core checks: pass against the correct service twice and against its correct variants, and fail against every planted bug.\n\n", len(shared), strings.Join(ids, ", "))

	var withAxx condStats
	withAxx.coreMisses = map[string]int{}
	var axxConds []string
	for _, c := range f.Conditions {
		if c == plainCondition {
			continue
		}
		axxConds = append(axxConds, c)
		s := st[c]
		withAxx.trials += s.trials
		withAxx.corePassed += s.corePassed
		withAxx.work.add(s.work)
		withAxx.agentSeconds += s.agentSeconds
		withAxx.costUSD += s.costUSD
		for k, v := range s.coreMisses {
			withAxx.coreMisses[k] += v
		}
	}
	plain := st[plainCondition]
	fmt.Fprintf(b, "**With axx, %d of %d suites passed every core check (%s)**, across %s; **without axx, %d of %d (%s)**.",
		withAxx.corePassed, withAxx.trials, pct(withAxx.corePassed, withAxx.trials), strings.Join(axxConds, ", "),
		plain.corePassed, plain.trials, pct(plain.corePassed, plain.trials))
	for _, side := range []struct {
		name string
		s    condStats
	}{{"With axx", withAxx}, {"Without axx", plain}} {
		var parts []string
		for _, why := range missOrder {
			if n := side.s.coreMisses[why]; n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", n, why))
			}
		}
		if len(parts) > 0 {
			fmt.Fprintf(b, " %s: %s.", side.name, strings.Join(parts, ", "))
		}
	}
	lw, okw := median(withAxx.work.Lines)
	lp, okp := median(plain.work.Lines)
	if okw && okp {
		fmt.Fprintf(b, " The agents wrote a median of %.0f lines per suite with axx and %.0f without.", lw, lp)
	}
	b.WriteString("\n\n")

	header(b, "On these tasks", f.Conditions)
	row(b, "**Passed every core check**", f.Conditions, func(c string) string {
		s := st[c]
		return fmt.Sprintf("**%d/%d**", s.corePassed, s.trials)
	})
	for _, why := range missOrder {
		any := false
		for _, c := range f.Conditions {
			any = any || st[c].coreMisses[why] > 0
		}
		if !any {
			continue
		}
		row(b, sentence(why), f.Conditions, func(c string) string { return fmt.Sprint(st[c].coreMisses[why]) })
	}
	row(b, "Lines written per suite (median)", f.Conditions, func(c string) string {
		v, ok := median(st[c].work.Lines)
		return num(v, ok, "%.0f")
	})
	row(b, "Agent minutes per trial", f.Conditions, func(c string) string {
		v, ok := st[c].perTrial(st[c].agentSeconds / 60)
		return num(v, ok, "%.1f")
	})
	row(b, "Requests learning axx per trial", f.Conditions, func(c string) string {
		v, ok := st[c].perTrial(float64(st[c].work.LearnRequests))
		return num(v, ok, "%.1f")
	})
	row(b, "Cost per trial (¢)", f.Conditions, func(c string) string {
		s := st[c]
		n := s.trials + s.unscored
		if n == 0 {
			return "-"
		}
		return fmt.Sprintf("%.2f", s.costUSD*100/float64(n))
	})
	row(b, "**Cost per passing suite (¢)**", f.Conditions, func(c string) string {
		s := st[c]
		if s.corePassed == 0 {
			return "-"
		}
		return fmt.Sprintf("**%.2f**", s.costUSD*100/float64(s.corePassed))
	})
	b.WriteString("\nLines written are the lines of the files the agent added or changed: features and seeds with axx, test code and scripts without. Requests learning axx are those whose tool calls mostly read its steps, docs, skills or help. The cost per passing suite is what every trial cost, divided by the suites that passed every core check. Every failed suite is listed under \"Every miss\" below.\n\n")
}

// howTheyWorked is what the agents did per scored trial, over every task.
func (f *File) howTheyWorked(b *strings.Builder) {
	st := map[string]condStats{}
	any := false
	for _, c := range f.Conditions {
		st[c] = f.stats(c, f.Tasks)
		any = any || st[c].work.ToolCalls > 0
	}
	if !any {
		return
	}
	b.WriteString("## How the agents worked\n\nPer scored trial, every task.\n\n")
	header(b, "Per trial", f.Conditions)
	for _, m := range []struct {
		name string
		v    func(s condStats) float64
	}{
		{"Tool calls", func(s condStats) float64 { return float64(s.work.ToolCalls) }},
		{"Model requests", func(s condStats) float64 { return float64(s.work.ModelRequests) }},
		{"axx commands", func(s condStats) float64 { return float64(s.work.AxxCommands) }},
		{"axx MCP tool calls", func(s condStats) float64 { return float64(s.work.MCPCalls) }},
		{"Skills loaded", func(s condStats) float64 { return float64(s.work.SkillsLoaded) }},
		{"Web pages fetched (docs)", func(s condStats) float64 { return float64(s.work.DocsFetched) }},
		{"Requests learning axx", func(s condStats) float64 { return float64(s.work.LearnRequests) }},
		{"Tokens read learning axx (thousands)", func(s condStats) float64 { return float64(s.work.LearnTokens) / 1000 }},
		{"Agent minutes", func(s condStats) float64 { return s.agentSeconds / 60 }},
		{"Input tokens (thousands)", func(s condStats) float64 { return float64(s.inputTokens) / 1000 }},
	} {
		row(b, m.name, f.Conditions, func(c string) string {
			v, ok := st[c].perTrial(m.v(st[c]))
			return num(v, ok, "%.1f")
		})
	}
	row(b, "Lines written (median)", f.Conditions, func(c string) string {
		v, ok := median(st[c].work.Lines)
		return num(v, ok, "%.0f")
	})
	row(b, "Cost (¢)", f.Conditions, func(c string) string {
		s := st[c]
		n := s.trials + s.unscored
		if n == 0 {
			return "-"
		}
		return fmt.Sprintf("%.2f", s.costUSD*100/float64(n))
	})
	b.WriteString("\n")
}

// stumbled counts the troubles the agents ran into with axx.
func (f *File) stumbled(b *strings.Builder) {
	var conds []string
	for _, c := range f.Conditions {
		if c != plainCondition {
			conds = append(conds, c)
		}
	}
	type cell struct{ trials, times, of int }
	counts := map[string]map[string]cell{}
	for _, name := range StumbleNames() {
		counts[name] = map[string]cell{}
	}
	any := false
	for _, c := range conds {
		for _, t := range f.Tasks {
			r := t.Results[c]
			if r == nil {
				continue
			}
			for _, name := range StumbleNames() {
				x := counts[name][c]
				x.of += r.Trials
				if s, ok := r.Stumbles[name]; ok {
					x.trials += s.Trials
					x.times += s.Times
					any = true
				}
				counts[name][c] = x
			}
		}
	}
	if !any {
		return
	}
	b.WriteString("## Where the agents stumbled\n\nTroubles in the output of the agents' axx commands and MCP tools: the trials they happened in, and the outputs that showed them (an output that names one several times counts once).\n\n")
	header(b, "Trials · times", conds)
	for _, name := range StumbleNames() {
		seen := false
		for _, c := range conds {
			seen = seen || counts[name][c].times > 0
		}
		if !seen {
			continue
		}
		row(b, sentence(name), conds, func(c string) string {
			x := counts[name][c]
			if x.times == 0 {
				return "-"
			}
			return fmt.Sprintf("%d/%d · %d", x.trials, x.of, x.times)
		})
	}
	b.WriteString("\n")
}

// axxOnlyMisses sums, per condition with axx, the trials that passed every
// core check but missed an axx-only one, by kind: where the agent aids show.
func (f *File) axxOnlyMisses(b *strings.Builder) {
	var parts []string
	kinds := map[string]bool{}
	total := 0
	for _, c := range f.Conditions {
		if c == plainCondition {
			continue
		}
		n := 0
		for _, t := range f.Tasks {
			if r := t.Results[c]; r != nil {
				for _, m := range r.Misses {
					if !m.Core {
						n++
						kinds[m.Why] = true
					}
				}
			}
		}
		total += n
		parts = append(parts, fmt.Sprintf("%s %d", c, n))
	}
	if total == 0 || len(parts) == 0 {
		return
	}
	var ks []string
	for k := range kinds {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	fmt.Fprintf(b, "Suites that passed every core check but missed an axx-only one: %s (%s).\n\n", strings.Join(parts, ", "), strings.Join(ks, "; "))
}

// everyMiss lists every scored trial that did not get reward 1.
func (f *File) everyMiss(b *strings.Builder) {
	var lines []string
	for _, c := range f.Conditions {
		tasks := append([]Task(nil), f.Tasks...)
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		for _, t := range tasks {
			r := t.Results[c]
			if r == nil {
				continue
			}
			for _, m := range r.Misses {
				kind := m.Why
				if !m.Core {
					kind += " (axx-only check)"
				}
				line := fmt.Sprintf("- `%s` (%s, %s): **%s**", t.ID, c, trialSuffix(m.Trial), kind)
				if m.Detail != "" {
					line += ". " + m.Detail
				}
				lines = append(lines, line)
			}
		}
	}
	if len(lines) == 0 {
		return
	}
	b.WriteString("## Every miss\n\nEach scored trial that did not get reward 1, and the first check it failed (core checks first).\n\n")
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n\n")
}

// trialSuffix is the trial's id after its task's name.
func trialSuffix(trial string) string {
	if i := strings.LastIndex(trial, "__"); i >= 0 {
		return trial[i+2:]
	}
	return trial
}
