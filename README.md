# axx agent evals

These evals measure how well coding agents write axx acceptance tests, with and without the
aids axx ships for agents: the skills (`axx skills install`), the MCP server (`axx mcp`) and
the `AGENTS.md` section (`axx init`); and how well they write acceptance tests without axx at
all, with tools of their choosing, to compare with. Each task gives an agent a small repository and
acceptance criteria written the way a product person writes them; a verifier then decides,
without looking at how the agent worked, whether the resulting tests are real.

The evals run on [Harbor](https://harborframework.com) (the successor of Terminal-Bench): every
task is a Harbor task directory. They evaluate one agent, [OpenCode](https://opencode.ai), with
several models through [OpenRouter](https://openrouter.ai), so a difference between two runs is
the model's (or axx's), not the agent's. OpenCode runs any model and reads all three aids from
where axx puts them: `AGENTS.md`, the skills in `.agents/skills` and the MCP server.

| Model (OpenRouter id) | Why |
| --- | --- |
| `anthropic/claude-sonnet-5.5` | what most people code with |
| `openai/gpt-6.1-sol` | OpenAI's workhorse |
| `google/gemini-3.1-pro-preview` | Google's newest Pro |
| `z-ai/glm-5.3` | a strong open-weight coding model |
| `openai/gpt-6-luna` | a small model: the aids should help weak models most; also the cheap shakedown |

The ids are pinned (never an alias such as `~google/gemini-pro-latest`), so runs stay comparable.

## What's here

| Path | What |
| --- | --- |
| `app/` | **parcels**, the system under test: a Go HTTP service (OpenAPI 3.1 at `/openapi.json`) with PostgreSQL storage, a MongoDB tracking read model, a manifest importer, calls to a downstream address service (WireMock in tests) and Avro `ParcelRegistered` events on Kafka. It can be started with deliberate bugs (mutants) or as a correct variant. `app/compose.yaml` runs it with its infrastructure. |
| `internal/mutant/` | The mutants, each a realistic bug a good acceptance test must catch, and the correct variants, each a way the service may differ within its contract that a good acceptance test must not trip over. |
| `workspace/` | The starting project most tasks share: README, `axx.yaml`, the OpenAPI contract, the business rules and the database schema in `docs/`, the WireMock stubs. |
| `workspace-plain/` | What the starting project without axx has instead: its README (and `.gitignore`). |
| `tasks/<id>/` | The tasks (Harbor format, below). |
| `cmd/evals-verify/`, `internal/verify/` | The verifier that runs inside the verifier container. |
| `cmd/evals/` | The runner: build images, prepare conditions, run Harbor, report, compare. |
| `images/` | The Docker images the tasks build on, and `images/base/axx.env`: the axx release under test. |
| `conditions.toml`, `conditions/mcp.json` | The conditions. |
| `results/` | Result files (`.json` and `.md`) and each model's baseline. |
| `testdata/negative/` | Weak or cheating answers, each of which must get reward 0. |

## The tasks

| Task | Needs | Criteria | Mutants it must catch | Variants it must pass |
| --- | --- | --- | --- | --- |
| `rest-crud-happy-path` | rest | register, look up, change, cancel a parcel | `wrong-status-on-create`, `update-not-persisted`, `delete-not-removed` | `json-properties-reordered` |
| `openapi-reject-invalid` | rest | overweight and unknown service level refused with 400; nothing stored (needs a deliberately relaxed OpenAPI validation level) | `no-weight-limit`, `no-service-level-check` | `json-properties-reordered` |
| `mock-address-check` | rest | the postcode check with the address service (WireMock): called once, with the API key; zone stored; undeliverable refused with 422 | `skips-address-check`, `address-check-without-api-key`, `ignores-undeliverable` | `json-properties-reordered` |
| `sql-manifest-import` | | seed manifest lines, assert the parcel rows (JSON columns) and the line status | `import-drops-postcode`, `import-leaves-line-pending`, `import-wrong-source`, `import-accepts-overweight` | `import-takes-seconds`, `json-properties-reordered` |
| `mongo-tracking-view` | | seed scans in MongoDB, assert the tracking read model | `tracking-status-from-first-scan`, `tracking-counts-duplicate-scans` | `tracking-takes-seconds`, `json-properties-reordered` |
| `kafka-registered-event` | rest, kafka | an Avro `ParcelRegistered` event is published, keyed and filled correctly | `event-not-published`, `event-weight-in-kilograms` | `events-topic-gzip`, `json-properties-reordered` |
| `fix-broken-feature` | | repair a feature with a wrong step text and a wrong expectation, keeping its scenarios | `import-wrong-source`, `import-accepts-overweight` | `import-takes-seconds`, `json-properties-reordered` |
| `init-first-feature` | rest | `axx init` a bare repository, make `axx run` start the service, write the first feature | `wrong-status-on-duplicate`, `wrong-status-on-create` | `json-properties-reordered` |
| `parallel-unique-data` | rest | a shop's parcel list, correct under 16 workers in random order, plus an `axx lint` rule that bites | `list-ignores-sender-filter`, `delete-not-removed` | `json-properties-reordered` |
| `registration-end-to-end` | rest, kafka | a registration followed through every service: the address check (once, with the API key), the stored parcel and its zone, the undeliverable refusal, the `ParcelRegistered` event | `wrong-status-on-create`, `skips-address-check`, `address-check-without-api-key`, `ignores-undeliverable`, `event-not-published`, `event-weight-in-kilograms` | `json-properties-reordered`, `events-topic-gzip` |
| `change-rules` | rest | no change or cancellation once a depot picked the parcel up (a status only the database holds, so the tests seed it), the refused change not stored, a registered parcel still changeable | `change-after-pickup`, `refused-change-stored`, `cancel-after-pickup`, `update-not-persisted` | `json-properties-reordered` |

The correct variants (`internal/mutant`) differ from the default service only where the docs and
the contract leave room, the way real services do:

| Variant | The change |
| --- | --- |
| `json-properties-reordered` | JSON response bodies list their properties in another order. |
| `import-takes-seconds` | Manifest lines are imported 3 seconds after they arrive, not at the next poll (the docs promise "within a few seconds"). |
| `tracking-takes-seconds` | The tracking view is updated 3 seconds after a parcel's latest scan arrives (the docs promise "within a few seconds"). |
| `events-topic-gzip` | The events topic has `compression.type=gzip`, so the broker stores every event in a gzip-compressed batch. |

A test that compares JSON as text, checks a background job before it had time, or reads Kafka
with a decoder that cannot decompress passes against the default service and fails a variant.

"Needs" lists the axx packs beyond core, mock, sql and mongo (`requires` in `task.toml`). The six
behavior tasks also have a version without axx (`tasks/<id>/plain`, below); setting axx up,
repairing a feature and the parallel workers with `axx lint` are axx by nature, so those three
have none. The
runner skips a task that needs a pack the axx release under test does not publish
(`axx pack list`), so a task can be written before its pack is released.

## Run the evals locally

You need Docker (with Compose v2), Go (see `go.mod`), and Harbor at the version CI uses
(`HARBOR_VERSION` in the workflows):

```sh
uv tool install harbor==0.23.0
mise install                      # the Infisical CLI, for the API key (mise.toml)
```

From this directory:

```sh
# Check the plumbing without any model, as CI does: the oracle agent applies each task's
# reference solution (every verifier must give 1), the nop agent does nothing (every verifier
# must give 0). --expect-reward fails the run otherwise.
go run ./cmd/evals run --agent oracle --conditions none --expect-reward 1
go run ./cmd/evals run --agent nop --conditions none --expect-reward 0

# Evaluate a model under every condition. The OpenRouter key comes from Infisical
# (.infisical.json links this checkout to its project); it costs model credits.
infisical run -- go run ./cmd/evals run --agent opencode --model openrouter/openai/gpt-6-luna \
  --ak version=1.18.34
```

`run` builds the images (with the axx release `images/base/axx.env` names: its version and the
checksums of its Linux archives, from the release's `checksums.txt`), writes one Harbor dataset per condition
under `.work/datasets/`, runs `harbor run` for each (jobs under `.work/jobs/`), and writes
`results/<date>-<agent>.json` and `.md`. A condition whose Harbor run fails does not stop the
others: the results keep its finished trials, and the run exits 1 afterwards.

A trial that something other than the agent ended runs again, up to twice, after 60 s and then
120 s: its containers failed to build or start (`RuntimeError`, which Harbor raises plain only
for infrastructure, `EnvironmentStartTimeoutError`, `HealthcheckError`), or the model provider's
rate limit cut the agent off (`ApiRateLimitError`). Harbor matches exact type names, so the
agent's own failures (`NonZeroAgentExitCodeError`, `AgentTimeoutError`) are never retried and
no attempt gets a second chance (`results.NotScored`; `run` writes Harbor's retry settings to
`.work/datasets/<run>-harbor.json`). A trial that still ends in one of them is **not scored**:
the results leave it out, list it, and the run (and `report`) exits 1, since the scores then rest
on fewer trials than asked for.

**Rate limits.** Every agent sends its model about 14 requests a minute, 22 at its busiest, and
the model provider may allow fewer: OpenRouter limits new accounts to 20 requests a minute per
model ("new accounts are limited to 20 requests per minute for this model", `limit_source:
openrouter_new_account`), which cut off 80 of 126 trials of the first parallel run. No rate is
set anywhere instead. For a model reached through OpenRouter, `run` serves a proxy on the host
(`internal/ratelimit`; Linux: the Docker bridge's gateway, Docker Desktop: the loopback) and
points OpenCode at it (Harbor's `opencode_config`: OpenRouter's `baseURL`, and a header with the
agent container's hostname). The proxy passes requests straight through; when the provider
answers 429, it holds every request until the limit resets (`X-RateLimit-Reset`, or
`Retry-After`, or a doubling backoff) and sends the refused one again, so no agent is cut off,
a run goes as fast as the limit allows, and at full speed when there is none. Refused requests
are not billed.

The waits are not the agent's: the proxy records each trial's (`<job>/rate-limit.json`), and the
results leave them out. **Agent minutes** are the agent's run less its waits; the task's time
budget (`[agent] timeout_sec`) holds the agent to that working time, so Harbor gets four times
the budget in wall-clock time (`--agent-timeout-multiplier 4`), an agent that worked longer
than its budget fails as if timed out, and one Harbor timed out only because of the waits is not
scored (`RateLimitWaits`). The report says how long the limit held the agents in all.
`go run ./cmd/evals proxy` serves the proxy on its own, to try it.

**One model.** Every model call of a trial goes to the model under test. OpenCode also uses a
"small model" for side tasks like the session's title, and by default picks a cheaper model of
the same provider; through OpenRouter that was `google/gemini-3.8-flash`, once per trial in the
first runs. `run` sets OpenCode's `small_model` to the model under test (`--small-model` names
another; `auto` leaves OpenCode's choice). The proxy counts each trial's requests by model, and
a run whose agents called any other model says so in its report and exits 1. Useful flags:
`--tasks a,b`, `--shard 2/4` (a quarter of the tasks, by their place in name order),
`--conditions none,both`, `--attempts 3` (Harbor's `-k`), `--concurrency 4` (each trial runs its
own databases, so budget about 4 GB of memory per concurrent trial), `--skip-images`, `--dry-run` (print the Harbor commands), `--include-pending` (run tasks whose
packs are missing), `--expect-reward`. Agent options pass through with `--ak key=value`; for
OpenCode, `version=` pins its release (the workflow's `OPENCODE_VERSION`). Any other Harbor agent
works with `--agent` too, but the results are only comparable with the same agent.

Step by step, or with your own Harbor flags:

```sh
go run ./cmd/evals images                                  # axx-evals-base, -verifier, -address-service
go run ./cmd/evals prepare --condition skills --out /tmp/ds-skills
harbor run -p /tmp/ds-skills -a opencode -m openrouter/openai/gpt-6-luna --ak version=1.18.34 \
  -o jobs --job-name skills
go run ./cmd/evals report --agent opencode --model openrouter/openai/gpt-6-luna --job skills=jobs/skills
```

A single task also runs straight from its directory once the images exist (it is a regular
Harbor task): `harbor run -p tasks/sql-manifest-import -a oracle`.

## The conditions

`conditions.toml` defines them; the runner applies them per task:

| Condition | axx | AGENTS.md section | Skills | MCP server |
| --- | --- | --- | --- | --- |
| `none` | yes | | | |
| `skills` | yes | yes | yes | |
| `mcp` | yes | yes | | yes |
| `both` | yes | yes | yes | yes |
| `plain` | no | | | |

- **AGENTS.md section**: the section `axx init` writes (`project-setup finish --agents-md` runs
  `axx init` in a scratch project and keeps its `AGENTS.md`). Every initialized axx project has
  it, so every condition except the bare baseline includes it; set `agents_md` to change that.
- **Skills**: `axx skills install` in the project, exactly as a user runs it (`.agents/skills`
  for OpenCode, Codex, Cursor, Gemini CLI and Copilot). A repository not set up for axx yet
  (`init-first-feature`) has no packs for the project's skills, so it gets them as a developer
  has them before `axx init`: `axx skills install --scope user`, in `~/.agents/skills`.
- **MCP**: `axx mcp`, registered through Harbor's `--mcp-config conditions/mcp.json` (a
  Claude-style `.mcp.json`), which Harbor turns into each agent's own MCP configuration (for
  OpenCode, the `mcp` section of its `opencode.json`).

The AGENTS.md section and the skills are baked into the project before its initial commit, so the
agent sees them as part of the repository.

### The plain condition

`plain` is the same job without axx, to compare with: tasks run their version in
`tasks/<id>/plain`. The agent gets the same acceptance criteria, the same running service and the
same docs, contract and stubs, but axx is not installed and the project has no `axx.yaml` or
`features/` (`project-setup plain`). It writes the tests with whatever it likes: Go, Node.js,
Python and the usual command-line tools are installed, and it may install more. The one rule:
`./acceptance-tests.sh` runs them all and exits 0 when they pass. The verifier runs it in
command mode (below) against the correct service and each mutant.

A plain version has its own `instruction.md` (the task's, with a tool-neutral last paragraph),
`tests/verify.toml` (`mode = "command"`) and reference solution (Go tests). `sync` generates the
rest, as for every task.

## The images

`go run ./cmd/evals images` builds three images from this repository:

- **`axx-evals-base`**, the agent's environment: the axx release `images/base/axx.env` names,
  downloaded from its GitHub release and checked against the SHA-256 there; the parcels service;
  and the common starting project. The packs of the starting projects are prepared at build
  time, in one cache every user shares (`XDG_CACHE_HOME=/var/cache/evals`), so no trial waits for
  axx to prepare them. A project that lists other packs prepares them on first use, as it would
  for any user.
- **`axx-evals-verifier`**: the base plus the verifier, for the separate verifier container only.
- **`axx-evals-address-service`**: WireMock with the service's stubs.
- **`axx-evals-mongo`**: MongoDB with the `parcels` user created when the image is built. The
  `mongo` image creates it at every start through a temporary server, which now and then still
  holds the port when the real one starts (exit 48) and fails the trial.

The other sidecars (PostgreSQL, Kafka, the Schema Registry) use the images of `app/compose.yaml`;
`go run ./cmd/evals sync` copies them into the tasks' compose files.

To test another axx release, set `AXX_VERSION` and the two `AXX_SHA256_*` lines in
`images/base/axx.env` from the release's `checksums.txt`, then rebuild the images and run the
plumbing. A release that renames steps breaks the reference solutions that use them: the oracle
run (and CI) says which.

## How a task is verified

Each task runs Harbor's **separate verifier** (`environment_mode = "separate"` in `task.toml`):
after the agent finishes, Harbor copies the agent's `/app` (without `.axx` and `.git`) into a
fresh container built from `tests/Dockerfile`, with fresh databases, and runs `tests/test.sh`,
which runs `evals-verify` with the task's `tests/verify.toml`. Nothing the agent did to its own
container (binaries, databases, the app) can reach the verifier, and the agent never sees the
verifier, its checks or the lists of mutants and variants.

The verifier writes two rewards. The **reward** is 1 only if every check passes:

1. **Static rules** (below): only allowed files changed, the features read as acceptance
   criteria, no probes, no fake custom steps.
2. **`axx validate`** reports no problems. Where the task asks: **`axx lint`** passes, and fails
   once the verifier plants a copy of one of the agent's seed files (the rule bites).
3. **The correct app passes**: `axx run` exits 0 with at least `min_scenarios` passing scenarios,
   nothing skipped, pending or undefined, every scenario tag filter overridden (a `@wip` tag does
   not hide a scenario), `--workers 8` and a random order, **twice** (`run.repeat`, default 2;
   more where the task asks), each time on fresh data in a new random order, so a flaky suite
   fails; `start_apps` makes the first run a plain `axx run` that starts the service from the
   agent's own `axx.yaml`; `preserve_scenarios` must still exist and pass.
4. **Every correct variant passes**: for each of the task's `variants` the verifier resets all
   data, starts the app as that variant and requires the same as on the correct app.
5. **Every targeted mutant fails**: for each mutant the verifier resets all data, starts the app
   with that bug, runs the suite and requires exit code 1 with at least one failing scenario. Exit
   codes 2 to 4 (configuration, undefined steps, app failures) do not count as catching the bug.

The **core reward** counts only what tests of any kind are held to: the `allowed-paths`,
`required`, `absent`, `must-not-contain` and `probe` rules, passing against the correct app and
every correct variant, and failing against every mutant. It leaves out what only an axx suite has: `axx validate`, the
features' readability rules, `config-keys`, `axx lint`, `min_scenarios` and
`preserve_scenarios`. Every condition, `plain` included, compares on it. The verifier runs the
suite even when one of those axx-only checks fails, so the core reward is always decided.

In **command mode** (`mode = "command"`, the plain condition) the agent's `command` (default
`./acceptance-tests.sh`, run with `bash`) stands in for `axx run`: on the correct app it must exit
0 (`run.repeat` times, on fresh data) and on each correct variant too, and on each mutant any
other exit, or a timeout (`run.timeout`), catches the bug. Any exit counts there, so the variants
are what keep a test from catching bugs by breaking: a test that fails for reasons of its own
(a decoder that cannot read a compressed batch, a check that does not wait for the importer)
fails a variant as well. `protected` lists the files the agent must leave as they are
(the README, the contract, the docs, the stubs, the schemas); everything else is `allowed`. The
two rewards are the same there.

The verifier starts and stops the app itself (as the `parcels` user, with `EVALS_MUTANT` and
`EVALS_VARIANT` in the app's own environment only) and runs the tests as the unprivileged `tester`
user, so nothing the suite runs can read which variant is running. Each run's logs stay in a
directory only root can read until the verifier finishes; only then are they copied to the
trial's `verifier/` directory, with `verify.json`, which holds every check with its details
(`core` marks the core ones), each run's report or output and the app logs; `reward.json` has
`reward` and `core`.

## Anti-cheat rules

| Rule | What it rejects |
| --- | --- |
| `allowed-paths` | Adding, changing or deleting any file outside the task's `allowed` globs (the service, the docs, the stubs, `axx.yaml` unless the task needs it). Agent state (`.git`, `.axx`, `.agents`, `.claude`, `.mcp.json`, `AGENTS.md`, `node_modules`, caches) is ignored. The files of axx's fixture factories (`*.factory.yaml`, `*.fixture.yaml`, and the manifest, pairings and lint rules `axx fixtures` writes) are allowed in every task; the files they generate must be in the task's `allowed` globs. |
| `config-keys` | Changing any top-level `axx.yaml` key other than the ones the task names (e.g. only `lint`). |
| `required`, `absent`, `must-not-contain` | Missing deliverables (`axx.yaml`, features) or leftovers the task forbids. |
| `variables` | `${var:...}` or `${env:...}` in a feature: data is chosen up front, never captured or passed between steps. |
| `doc-string-code` | Doc strings with code (anything but a payload: JSON, XML, YAML, CSV, text). |
| `scenario-name`, `no-outcome` | Unnamed or duplicate scenarios, and scenarios without a `Then` step. |
| `probe` | Code that tries to detect the variant under test instead of testing behavior: references to `EVALS_MUTANT`, `EVALS_VARIANT`, "mutant", `/proc/`, `/opt/evals`, `/logs/verifier` in sources the agent writes. |
| mutant checks | Tests that pass no matter what (fake steps, missing assertions, assertions on the mock instead of the service) survive the mutants and get reward 0; tests that fail no matter what never pass the correct app. |
| variant checks | Tests that depend on what the contract leaves open (JSON property order, how fast the importer or the tracking view is, how the broker stores events) fail a correct variant and get reward 0, even when they happen to fail every mutant. |

The mutant and variant checks are the core of it: a suite earns its reward only by passing
against the correct service and its correct variants, and failing against each deliberately
broken one.

## Results and the release gate

`results/<date>-<agent>.json` (schema below) and a Markdown report are written after every run
(in CI, the report is the run's summary). A condition's **score** is the mean reward over the
tasks that ran, in percent, and its **core score** the mean core reward: the one to compare
conditions on, `plain` included. The report is meant to be read on its own; everything in it comes
from the trials (their results, transcripts, verifier reports and the files the agents wrote):

- **With axx and without**: on the tasks `plain` ran, how many suites passed every core check
  under each condition, how the others failed (against the correct service, a correct variant,
  a missed bug), the lines the agents wrote per suite, their minutes, the model requests they
  spent learning axx (reading its steps, docs, skills or help), the cost per trial, and the cost
  per passing suite (every trial's cost over the suites that passed every core check).
- **Scores by task**: the task x condition table, the core score and the score, and the suites
  that passed every core check but missed an axx-only one, where the agent aids show.
- **How the agents worked**: per trial, tool calls, model requests, axx commands, axx MCP tool
  calls, skills loaded, web pages fetched, the requests and tokens spent learning axx, agent
  minutes, input tokens, lines written and cost.
- **Where the agents stumbled**: troubles in the output of the agents' axx commands and MCP tools
  (a service used before it was registered, a seed the database rejected, a property set inside
  an object the payload lacks, requests numbered out of order, undefined step text, unknown step
  ids, unknown commands, payload table values in single quotes, a step checking the first
  selection after a later one, a lint rule that finds no values), in trials and in the outputs
  that showed them.
- **Every miss**: each scored trial that did not get reward 1, with the first check it failed.
- Run health: trials not scored, other models called, the rate limit's waits.

```json
{
  "schemaVersion": 1,
  "kind": "axx-evals-results",
  "date": "2026-10-02",
  "agent": "opencode",
  "model": "openrouter/openai/gpt-6-luna",
  "axx": "0.1.12",
  "harbor": "0.23.0",
  "conditions": ["none", "skills", "mcp", "both", "plain"],
  "tasks": [
    {"id": "rest-crud-happy-path", "title": "...", "category": "rest",
     "results": {"none": {
       "reward": 0.33, "core": 1, "trials": 3, "passed": 1, "corePassed": 3,
       "mutantsCaught": 9, "mutantsTotal": 9,
       "agentSeconds": 238.6, "rateLimitWaitSeconds": 412.0, "rateLimitRefused": 7,
       "models": {"openai/gpt-6-luna": 71}, "costUsd": 0.0389, "inputTokens": 1536000, "outputTokens": 9640,
       "work": {"toolCalls": 156, "modelRequests": 71, "axxCommands": 87, "mcpCalls": 0, "skillsLoaded": 0, "docsFetched": 4, "lines": [59, 55, 61], "learnRequests": 15, "learnTokens": 54000},
       "stumbles": {"used a service before registering it": {"trials": 1, "times": 3}},
       "misses": [{"trial": "rest-crud-happy-path__8BLohbN", "why": "fewer scenarios than the task has criteria", "core": false,
                   "detail": "at least 3 passing scenarios: 1 passed"}]}}}
  ],
  "skipped": [{"id": "init-first-feature", "condition": "plain", "reason": "no version without axx (tasks/init-first-feature/plain)", "byDesign": true}],
  "scores": {"none": 58.3, "skills": 75.0, "mcp": 66.7, "both": 83.3, "plain": 50.0},
  "coreScores": {"none": 66.7, "skills": 83.3, "mcp": 75.0, "both": 83.3, "plain": 50.0}
}
```

Per task and condition: `trials` are the scored trials, `passed` those with reward 1 and
`corePassed` those that passed every core check; `errors` count scored trials that ended in the
agent's own exception, `unscored` (by exception type) those left out. `agentSeconds` is the
agents' working time, the rate limit's waits (`rateLimitWaitSeconds`, `rateLimitRefused`) left out;
`models` counts the model requests by model. `work` is what the agents did (`lines`: the lines of
the files each trial added or changed; `learnRequests` and `learnTokens`: the model requests spent
learning axx and the tokens they read), `stumbles` the troubles in axx's output by kind, and
`misses` every scored trial without reward 1 and the first check it failed.

The **baseline** is a result file promoted as-is, one per model: `results/baseline-<model>.json`,
with the model's OpenRouter id after `openrouter/` and `/` as `-` (copy a run you trust). The
gate compares a new result file with it:

```sh
go run ./cmd/evals compare results/baseline-openai-gpt-6-luna.json results/ci-openai-gpt-6-luna.json --max-drop 10
```

It scores both files over the tasks they have in common (so adding or skipping tasks does not
move the score), prints the per-condition change and the tasks that flipped, and exits 1 when any
condition dropped by more than `--max-drop` points (default 10). The `evals` workflow applies it
when the model has a baseline; `results/baseline-openai-gpt-6-luna.json` is the 0.1.12 run.

**The release gate:** axx's `release-gate` workflow runs the evals on release-please's pull
request before a release is published. It starts this repository's `evals` workflow with GPT-6
Luna and `axx_ref` set to the pull request's commit, so the images build axx from that commit
(`AXX_REF`, `images/base/Dockerfile`) rather than install a release, and it fails when the gate
does. It needs `AXX_EVALS_TOKEN` in axx, a token that may start workflows here; without it the
gate is off and says so.

**The results page:** axx's docs publish the latest results at
[Agent evaluations](https://axx.nimbusxr.us/explanations/agent-evals/), written from the result
files, one per model:

```sh
go run ./cmd/evals page -out ../axx/docs/src/content/docs/explanations/agent-evals.md \
  -link openrouter/openai/gpt-6-luna=https://github.com/nimbusxr/axx-evals/actions/runs/<run> \
  results/ci-openai-gpt-6-luna.json
```
Agents are nondeterministic: use `--attempts 3` for gate runs so a single unlucky trial does not
move a task by a full 100 points.

## CI

Two workflows:

- **`check`** runs on every pull request and push to `main`, with no secrets and no model: the Go
  tests (including the generated task files being up to date), actionlint and zizmor on the
  workflows, and the plumbing through Harbor, split across five jobs that run at once and a
  `plumbing` job that passes when they all do. Four take a quarter of the tasks each
  (`--shard`): the oracle agent must get reward 1 on every task, with axx and without (`none` and
  `plain`), the nop agent 0. The fifth builds every task's environment under every aid
  condition, runs the oracle again under each on `rest-crud-happy-path` and
  `init-first-feature` (the shared starting project and a bare one), checks that a container
  reaches the rate-limit proxy, and that every answer in `testdata/negative/` gets 0. Nothing
  costs money.
- **`evals`** runs a model on demand only (`workflow_dispatch`: pick the model, conditions, tasks
  and attempts, and optionally `axx_ref`, an axx commit to test instead of the release), from
  `main` only, never on pull requests. It runs OpenCode, one job per
  condition, all at once on their own runners, two trials at a time in each, with the
  rate-limit proxy pacing the model requests (see "Rate limits" above), then a
  report job merges them into `results/ci-<model>.json` and `.md`, adds the table to the run
  summary and applies the gate when the model has a baseline. The Harbor jobs (every trial's
  logs and trajectory) are kept as artifacts for 90 days.

The `OPENROUTER_API_KEY` secret lives in the `evals` environment, which only `main` can use. It is
not set by hand: Infisical (project `axx-evals`, environment `prod`) is where it lives, and its
`prod-to-github` sync writes it to that environment whenever it changes. The key's credit limit
on OpenRouter caps what runs can spend.

## Add a task

1. Create `tasks/<id>/` with:
   - `task.toml`: copy one. Set `[task].name = "axx-evals/<id>"`, and under `[metadata]` the
     `title`, `difficulty`, `category`, `requires` (packs beyond core, mock, sql, mongo),
     `services` (`postgres`, `mongo`, `address-service`, `kafka`) and `workspace` (`common`
     starts from `workspace/`; `none` starts from the overlay only). Keep
     `environment_mode = "separate"` and the `/app` artifact.
   - `instruction.md`: acceptance criteria as a product person writes them. Never step text.
   - `environment/workspace/`: files added to (or replacing) the starting project.
   - `tests/verify.toml`: `allowed`, `min_scenarios`, `mutants`, `variants` (every variant that
     touches what the task tests) and any other rule from `internal/spec/spec.go`.
   - `solution/solve.sh` (and files): the reference solution, written as acceptance criteria a
     person can read.
2. If no existing mutant fits, add one to `internal/mutant/mutant.go` and implement it in `app/`
   (every mutant must be targeted by some task; `go test ./...` checks). The same goes for a
   correct variant, which must stay within what `workspace/docs/` and `openapi.yaml` promise.
3. `go run ./cmd/evals sync` generates the task's Dockerfiles, compose files, `test.sh` and the
   initial-state manifest (`go test ./...` fails while they are stale).
4. For a version without axx, add `plain/` with `instruction.md`, `tests/verify.toml`
   (`mode = "command"`, `allowed = ["**"]`, the `protected` files, `required =
   ["acceptance-tests.sh"]`, the same `mutants` and `variants`) and `solution/`; `sync` writes the
   rest.
5. `go run ./cmd/evals check --solution <id>` must print reward 1 and
   `go run ./cmd/evals check --nop <id>` reward 0 (`--images` rebuilds the images first). `check`
   is Harbor in miniature: it starts the task's infrastructure with docker compose, builds the
   starting project in a verifier container and runs the verifier there, which is much faster
   while you iterate. Replay a weak or cheating answer with `--patch DIR` (it must get 0; see
   `testdata/negative/`). `--plain` checks the version without axx. Then run the task through
   Harbor with the oracle and nop agents (`go run ./cmd/evals run --agent oracle --tasks <id>
   --conditions none,plain --expect-reward 1`, and `nop` with `--expect-reward 0`).

## The app

```sh
docker compose -f app/compose.yaml up -d --build --wait                  # app + PostgreSQL, MongoDB, WireMock
docker compose -f app/compose.yaml --profile kafka up -d --build --wait  # plus Kafka and the Schema Registry
EVALS_MUTANT=no-weight-limit docker compose -f app/compose.yaml up -d app   # with a bug
```

`parcels reset` wipes every store (the Postgres schema, the Mongo database, the WireMock journal
and the events topic). The app's own tests: `go test ./...` in this directory.

## License

Apache-2.0, © NimbusXR. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
