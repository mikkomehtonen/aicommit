# AGENTS.md

CRITICAL RULES - MUST FOLLOW
RESPONSES

    Keep responses concise and to the point - unless the user asks otherwise

PLANNING MODE

    Always ask clarifying questions
    Never assume design, tech stack or features
    Use deep-dive sub-agents to assist with research
    Use deep-dive sub-agents to review the different aspects of your plan before presenting to the user

CHANGE / EDIT MODE

    Never implement features yourself when possible - use sub-agents!
    Identify changes from the plan that can be implemented in parallel, and use sub-agents to implement the features efficiently
    When using sub-agents to implement features, act as a coordinator only
    Use the best model for the task - premium models for complex tasks (like coding) and mid-tier models for simpler tasks, like documentation
    After completing features (large or small), always run commands like lint, type check and next build to check code quality

## Build & Run

```bash
go build ./cmd/aicommit   # build binary
go vet ./...              # static analysis
go test ./...             # run tests
```

Module name is `aicommit` (no remote path). Single binary, no subcommands.

## Architecture

```
cmd/aicommit/main.go     → Cobra root command, wires packages together
internal/git/diff.go     → git.StagedDiff() — shells out to `git diff --staged`
internal/llm/client.go   → llm.Generate(prompt) — HTTP POST to OpenAI-compatible API
internal/prompt/template.go → prompt.Build(diff) — injects diff into system prompt
```

Flow: `git diff` → `prompt.Build` → `llm.Generate` → stdout.

## LLM Client Details

- Uses **OpenAI-compatible** chat completions format (`/v1/chat/completions`), not Ollama's `/api/generate`.
- Endpoint default: `http://localhost:1234/v1/chat/completions`
- URL override: set `AICOMMIT_URL` env var
- Model override: set `AICOMMIT_MODEL` env var. Default: `qwen/qwen3.6-27b`
- Response is extracted from `choices[0].message.content`
- Requires an OpenAI-compatible LLM API server running locally with a model loaded

## Conventions

- Tests use fakes and `httptest` — no real git repo or LLM server required.
- No config files — model override is env-var only.
- The app prints only the commit message to stdout; errors go to stderr.
- Empty staged diff prints a hint to stderr and exits 1 (not via Cobra error handling).