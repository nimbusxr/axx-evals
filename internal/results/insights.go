package results

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Work is what the agents did, summed over a task's scored trials under one
// condition: from each trial's transcript (agent/trajectory.json), the
// rate-limit proxy's count of its model requests and the files it wrote.
type Work struct {
	ToolCalls     int `json:"toolCalls"`
	ModelRequests int `json:"modelRequests"`
	// AxxCommands are shell commands that run axx; MCPCalls are calls of the
	// axx MCP server's tools; SkillsLoaded, the agent skills it loaded;
	// DocsFetched, the web pages it fetched.
	AxxCommands  int `json:"axxCommands"`
	MCPCalls     int `json:"mcpCalls"`
	SkillsLoaded int `json:"skillsLoaded"`
	DocsFetched  int `json:"docsFetched"`
	// Lines are the lines of the files the agent added or changed, per trial
	// (the verifier's list of changes), in trial order.
	Lines []int `json:"lines,omitempty"`
}

func (w *Work) add(o Work) {
	w.ToolCalls += o.ToolCalls
	w.ModelRequests += o.ModelRequests
	w.AxxCommands += o.AxxCommands
	w.MCPCalls += o.MCPCalls
	w.SkillsLoaded += o.SkillsLoaded
	w.DocsFetched += o.DocsFetched
	w.Lines = append(w.Lines, o.Lines...)
}

// Stumble counts one kind of trouble the agents ran into with axx: the
// trials it happened in, and the outputs of axx commands and MCP tools that
// showed it (one output that names it in several places counts once).
type Stumble struct {
	Trials int `json:"trials"`
	Times  int `json:"times"`
}

// stumbleKinds are the troubles the report counts, in the output of the
// agents' axx commands and MCP tools (never in what the agents wrote or
// thought). Each names what happened, for a person reading the report.
var stumbleKinds = []struct {
	Name string
	re   *regexp.Regexp
}{
	// At runtime, or from validate and lint before it (AXX-E0836, since nimbusxr/axx#90).
	{"used a service before registering it", regexp.MustCompile(`No (?:database |MongoDB |Kafka )?services? set|(?:Service|Database service|MongoDB service) \\?"[^"\\]+\\?" not set|AXX-E0836`)},
	{"seed rejected by the database", regexp.MustCompile(`Could not perform (?:MongoDB )?seed`)},
	// axx 0.1.9 explains it ("the request payload has no recipient, so
	// recipient.name cannot be set inside it: … PathNotFoundException");
	// before, the bare exception was all.
	{"set a property inside an object the payload lacks", regexp.MustCompile(`PathNotFoundException`)},
	{"numbered requests (1st, 2nd …) out of order", regexp.MustCompile(`request of service \S+, (?:but only|which an earlier step)`)},
	{"undefined step text", regexp.MustCompile(`undefined step`)},
	{"no step with that id", regexp.MustCompile(`no step (?:has the id|with id)`)},
	{"axx command or flag that doesn't exist", regexp.MustCompile(`unknown (?:command|flag)[^\n]{0,60}for \\?"axx`)},
	{"payload table value in single quotes", regexp.MustCompile(`AXX-E0834`)},
	{"checked the first selection after a later one", regexp.MustCompile(`AXX-E0835`)},
	{"lint rule that finds no values", regexp.MustCompile(`AXX-E0821`)},
}

// StumbleNames lists the stumble kinds in report order.
func StumbleNames() []string {
	out := make([]string, len(stumbleKinds))
	for i, k := range stumbleKinds {
		out[i] = k.Name
	}
	return out
}

// Miss is a scored trial that did not get reward 1, and why: the first check
// it failed, core checks first.
type Miss struct {
	Trial string `json:"trial"`
	// Why groups misses: failedCorrect, failedVariant, missedBug, ... (see
	// missKinds), or the exception that ended the trial.
	Why string `json:"why"`
	// Core: a core check failed (else only an axx-only one did).
	Core   bool   `json:"core"`
	Detail string `json:"detail,omitempty"`
}

// Kinds of misses, as Miss.Why.
const (
	missFailedCorrect = "failed against the correct service"
	missFailedVariant = "failed against a correct variant"
	missMissedBug     = "missed a planted bug"
	missStatic        = "broke the task's rules"
	missNoChange      = "changed nothing"
	missScenarios     = "fewer scenarios than the task has criteria"
	missValidate      = "axx validate found problems"
	missReadability   = "features don't read as acceptance criteria"
	missLint          = "axx lint"
	missKeep          = "dropped a scenario it had to keep"
	missOverBudget    = "worked past its time budget"
	missTimedOut      = "timed out"
	missCrashed       = "the agent's process failed"
	missOther         = "other"
)

// classifyCheck names the kind of miss a failed check is.
func classifyCheck(name string) string {
	switch {
	case strings.HasPrefix(name, "passes against the correct variant"):
		return missFailedVariant
	case strings.HasPrefix(name, "passes against the correct app"):
		return missFailedCorrect
	case strings.HasPrefix(name, "catches mutant"):
		return missMissedBug
	case name == "static rules":
		return missStatic
	case name == "the agent changed the project":
		return missNoChange
	case strings.HasPrefix(name, "at least ") && strings.HasSuffix(name, "passing scenarios"):
		return missScenarios
	case name == "axx validate":
		return missValidate
	case name == "readability rules":
		return missReadability
	case strings.Contains(name, "lint"):
		return missLint
	case strings.HasPrefix(name, "keeps "):
		return missKeep
	}
	return missOther
}

// trialInsight is what one trial's files tell.
type trialInsight struct {
	work     Work
	stumbles map[string]int
	miss     *Miss // set when the verifier failed it
}

// verifyReport is the part of the verifier's verify.json the report reads.
type verifyReport struct {
	Checks []struct {
		Name   string `json:"name"`
		Passed bool   `json:"passed"`
		Core   bool   `json:"core"`
		Detail string `json:"detail"`
	} `json:"checks"`
	Static *struct {
		Changes []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"changes"`
	} `json:"static"`
}

// trajectory is the part of Harbor's ATIF transcript the report reads.
type trajectory struct {
	Steps []struct {
		Source    string `json:"source"`
		ToolCalls []struct {
			ID        string          `json:"tool_call_id"`
			Function  string          `json:"function_name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"tool_calls"`
		Observation *struct {
			Results []struct {
				CallID  string          `json:"source_call_id"`
				Content json.RawMessage `json:"content"`
			} `json:"results"`
		} `json:"observation"`
	} `json:"steps"`
}

var axxCommand = regexp.MustCompile(`(^|[\s;&|(])axx(\s|$)`)

// analyzeTrial reads a trial's transcript, its verifier report and the files
// it wrote. requests is the proxy's count of its model requests (0: count the
// agent's turns in the transcript).
func analyzeTrial(dir string, requests int) trialInsight {
	in := trialInsight{stumbles: map[string]int{}}
	turns := 0
	if b, err := os.ReadFile(filepath.Join(dir, "agent", "trajectory.json")); err == nil {
		var t trajectory
		if json.Unmarshal(b, &t) == nil {
			for _, s := range t.Steps {
				if s.Source == "agent" {
					turns++
				}
				outputs := map[string]string{}
				if s.Observation != nil {
					for _, r := range s.Observation.Results {
						outputs[r.CallID] = string(r.Content)
					}
				}
				for _, tc := range s.ToolCalls {
					in.work.ToolCalls++
					isAxx := false
					switch {
					case strings.HasPrefix(tc.Function, "axx_"):
						in.work.MCPCalls++
						isAxx = true
					case tc.Function == "skill":
						in.work.SkillsLoaded++
					case tc.Function == "webfetch":
						in.work.DocsFetched++
					case tc.Function == "bash":
						var a struct {
							Command string `json:"command"`
						}
						if json.Unmarshal(tc.Arguments, &a) == nil && axxCommand.MatchString(a.Command) {
							in.work.AxxCommands++
							isAxx = true
						}
					}
					if !isAxx {
						continue
					}
					out := outputs[tc.ID]
					for _, k := range stumbleKinds {
						if k.re.MatchString(out) {
							in.stumbles[k.Name]++
						}
					}
				}
			}
		}
	}
	in.work.ModelRequests = requests
	if requests == 0 {
		in.work.ModelRequests = turns
	}

	var v verifyReport
	if b, err := os.ReadFile(filepath.Join(dir, "verifier", "verify.json")); err == nil && json.Unmarshal(b, &v) == nil {
		lines := 0
		if v.Static != nil {
			for _, c := range v.Static.Changes {
				if c.Kind == "deleted" {
					continue
				}
				if b, err := os.ReadFile(filepath.Join(dir, "artifacts", "app", filepath.FromSlash(c.Path))); err == nil && !bytes.Contains(b, []byte{0}) {
					lines += bytes.Count(b, []byte("\n"))
					if len(b) > 0 && b[len(b)-1] != '\n' {
						lines++
					}
				}
			}
		}
		in.work.Lines = []int{lines}
		// The first failed check, core checks first.
		for _, core := range []bool{true, false} {
			for _, c := range v.Checks {
				if !c.Passed && c.Core == core {
					in.miss = &Miss{Why: classifyCheck(c.Name), Core: core, Detail: oneLine(c.Name + ": " + c.Detail)}
					break
				}
			}
			if in.miss != nil {
				break
			}
		}
	}
	return in
}

// oneLine shortens a check's detail to one line for the report.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}
