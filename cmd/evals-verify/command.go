package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runCommand verifies tests of any kind (mode = "command"): the agent's
// command must pass against the correct app (run.repeat times, on fresh data
// each time) and against every correct variant, and fail against every mutant
// the task targets. Every check here is a core check.
func (v *verifier) runCommand(ctx context.Context) error {
	script := strings.Fields(v.spec.Command)
	if len(script) == 0 {
		return fmt.Errorf("verify.toml: no command")
	}
	if p := filepath.Join(v.opt.workspace, script[0]); strings.HasPrefix(script[0], "./") {
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			v.check("the tests run with "+v.spec.Command, false, script[0]+" does not exist")
			return nil
		}
		// The script needs no executable bit from the agent: bash runs it.
		script = append([]string{"bash", p}, script[1:]...)
	}

	// The correct app.
	for i := 1; i <= v.spec.Run.Repeat; i++ {
		label, name := "correct", "passes against the correct app"
		if v.spec.Run.Repeat > 1 {
			label = fmt.Sprintf("correct-%d", i)
			name = fmt.Sprintf("passes against the correct app (run %d of %d, fresh data)", i, v.spec.Run.Repeat)
		}
		res, err := v.commandRun(ctx, label, "", "", script)
		if err != nil {
			return v.fail(name, err)
		}
		if !v.check(name, res.ExitCode == 0, describeCommand(res)) {
			return nil
		}
	}

	// The correct variants: the tests must pass on each. A failure fails the
	// core reward; the mutants still run.
	for _, n := range v.spec.Variants {
		res, err := v.commandRun(ctx, "variant-"+n, "", n, script)
		ok := err == nil && res.ExitCode == 0
		v.report.Variants[n] = ok
		detail := ""
		if err != nil {
			detail = err.Error()
		} else {
			detail = describeCommand(res)
		}
		v.check(variantCheck(n), ok, detail)
	}

	// Mutants: any failure of the tests catches one.
	for _, m := range v.spec.Mutants {
		kill := &Kill{}
		v.report.Mutants[m] = kill
		res, err := v.commandRun(ctx, "mutant-"+m, m, "", script)
		switch {
		case err != nil:
			kill.Detail = err.Error()
		case res.ExitCode == 0:
			kill.Detail = "the tests passed (" + describeCommand(res) + ")"
		default:
			kill.Killed = true
			kill.Detail = describeCommand(res)
		}
		v.check("catches mutant "+m, kill.Killed, kill.Detail)
	}
	v.done = true
	return nil
}

func (v *verifier) fail(name string, err error) error {
	v.check(name, false, err.Error())
	return nil
}

// commandRun resets the data, starts the app (as a mutant when m is set, as a
// correct variant when variant is), runs the command as the tester user and
// stops the app.
func (v *verifier) commandRun(ctx context.Context, label, m, variant string, script []string) (*RunResult, error) {
	if err := v.reset(ctx); err != nil {
		return nil, err
	}
	app, err := v.startApp(ctx, label, m, variant)
	if err != nil {
		return nil, err
	}
	defer v.stopApp(app)
	start := time.Now()
	out, code, err := v.asTester(ctx, v.spec.Run.Timeout.Duration, v.opt.workspace, script[0], script[1:]...)
	_ = os.WriteFile(filepath.Join(v.logs, "run-"+label+".log"), out, 0o644)
	r := &RunResult{Label: label, Mutant: m, Variant: variant, ExitCode: code, Seconds: time.Since(start).Seconds(), Error: tail(string(out), 600)}
	if err != nil {
		// A timeout: the tests did not pass.
		r.ExitCode, r.Error = -1, err.Error()+"; "+tail(string(out), 400)
	}
	v.report.Runs = append(v.report.Runs, r)
	return r, nil
}

func describeCommand(r *RunResult) string {
	s := fmt.Sprintf("exit %d after %.0fs", r.ExitCode, r.Seconds)
	if r.ExitCode != 0 && r.Error != "" {
		s += ": " + r.Error
	}
	return s
}
