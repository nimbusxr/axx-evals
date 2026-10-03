package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nimbusxr/axx-evals/internal/results"
)

// cmdPage writes the results page of axx's docs site
// (docs/src/content/docs/explanations/agent-evals.md in nimbusxr/axx) from
// result files, one per model: `evals page -out agent-evals.md -link
// MODEL=RUN_URL results/ci-*.json`.
func cmdPage(args []string) error {
	fs := flag.NewFlagSet("page", flag.ExitOnError)
	out := fs.String("out", "", "the page to write (default: stdout)")
	var links multiFlag
	fs.Var(&links, "link", "MODEL=URL of the model's run, like openrouter/openai/gpt-6-luna=https://github.com/... (repeatable)")
	_ = fs.Parse(args)
	if fs.NArg() == 0 {
		return exitError{2, "give the result files, one per model"}
	}
	var files []*results.File
	for _, p := range fs.Args() {
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		var f results.File
		if err := json.Unmarshal(b, &f); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		files = append(files, &f)
	}
	byModel := map[string]string{}
	for _, l := range links {
		m, u, ok := strings.Cut(l, "=")
		if !ok {
			return exitError{2, "--link takes MODEL=URL"}
		}
		byModel[m] = u
	}
	page := results.Page(files, byModel)
	if *out == "" {
		_, err := fmt.Print(page)
		return err
	}
	return os.WriteFile(*out, []byte(page), 0o644)
}
