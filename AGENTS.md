# AGENTS.md

## Build & Run

```bash
go build ./cmd/aicommit   # build binary
go vet ./...              # static analysis (no test suite yet)
```

Module name is `aicommit` (no remote path). Single binary, no subcommands.

## Architecture

```
cmd/aicommit/main.go     → Cobra root command, wires packages together
internal/git/diff.go     → git.StagedDiff() — shells out to `git diff --staged`
internal/llm/client.go   → llm.Generate(prompt) — HTTP POST to LM Studio
internal/prompt/template.go → prompt.Build(diff) — injects diff into system prompt
```

Flow: `git diff` → `prompt.Build` → `llm.Generate` → stdout.

## LLM Client Details

- Uses **OpenAI-compatible** chat completions format (`/v1/chat/completions`), not Ollama's `/api/generate`.
- Endpoint default: `http://localhost:1234/v1/chat/completions`
- URL override: set `AICOMMIT_URL` env var
- Model override: set `AICOMMIT_MODEL` env var. Default: `qwen/qwen3.6-27b`
- Response is extracted from `choices[0].message.content`
- Requires LM Studio running locally with a model loaded

## Conventions

- No tests yet — don't generate them unless asked.
- No config files — model override is env-var only.
- The app prints only the commit message to stdout; errors go to stderr.
- Empty staged diff prints a hint to stderr and exits 1 (not via Cobra error handling).