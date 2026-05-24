# aicommit

Generate Conventional Commit messages using a local LLM.

aicommit reads your git diff, sends it to an OpenAI-compatible LLM API, and prints a Conventional Commit message to stdout. By default it uses staged changes only; pass `--all` to include all changes.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- An OpenAI-compatible LLM API server (e.g. [LM Studio](https://lmstudio.ai/), [Ollama](https://ollama.com/), [llama.cpp](https://github.com/ggml-org/llama.cpp))

## Install

```bash
go install ./cmd/aicommit
```

Or build directly:

```bash
go build -o aicommit ./cmd/aicommit
```

To set a custom version number (shown with `--version`):

```bash
go build -ldflags "-X aicommit/cmd/aicommit.version=1.0.0" -o aicommit ./cmd/aicommit
```

## Usage

1. Stage your changes:

```bash
git add <files>
```

2. Run aicommit:

```bash
aicommit
```

The generated commit message is printed to stdout. Use it however you like:

```bash
aicommit | git commit -F -
```

### All changes (`--all`)

By default, aicommit only considers staged changes (`git diff --staged`). Add `--all` (or `-a`) to include all changes — both staged and unstaged (`git diff HEAD`):

```bash
aicommit --all
```

This is useful when you want a commit message that covers your full working tree diff, not just what's staged.

When combined with `--commit`, the accept action uses `git commit -am` instead of `git commit -m`, so unstaged changes to tracked files are automatically staged before committing.

### Interactive commit (`--commit`)

Add `--commit` (or `-c`) to review the generated message and commit interactively:

```bash
aicommit --commit
```

You'll see the suggested message and a prompt:

```
feat: add user authentication
[a]ccept, [e]dit, [r]etry, [c]ancel:
```

- **`a` or Enter** — accept and commit. Uses `git commit -m` by default, or `git commit -am` when `--all` is active
- **`e`** — edit the message; type a new message and press Enter, or press Enter with no input to keep the current one
- **`r`** — regenerate a new message from the same diff
- **`c`** — cancel without committing

## Configuration

The app connects to an OpenAI-compatible API at `http://localhost:1234` using the model `qwen/qwen3.6-27b`. Both can be overridden with environment variables:

```bash
AICOMMIT_URL=http://localhost:1234/v1/chat/completions AICOMMIT_MODEL=my-model aicommit
```

### Temperature

Control the LLM's randomness with two settings:

| Setting | Default | Env Var | CLI Flag |
|---------|---------|---------|----------|
| First request | `0.1` | `AICOMMIT_TEMPERATURE` | `--temperature` |
| Retry requests | `0.4` | `AICOMMIT_RETRY_TEMPERATURE` | `--retry-temperature` |

Lower temperature produces more deterministic output; higher temperature increases creativity and variety:

```bash
AICOMMIT_TEMPERATURE=0.2 AICOMMIT_RETRY_TEMPERATURE=0.6 aicommit --commit
aicommit --temperature 0.2 --retry-temperature 0.6 --commit
```

### API Key

Some OpenAI-compatible servers require an `Authorization` header. Set `AICOMMIT_API_KEY` to send a `Bearer` token:

```bash
AICOMMIT_API_KEY=sk-your-key aicommit
```

### Timeout

The default HTTP timeout is `60s`. Override it with a Go duration string (`30s`, `2m`, `1m30s`, etc.):

| Default | Env Var | CLI Flag |
|---------|---------|----------|
| `60s` | `AICOMMIT_TIMEOUT` | `--timeout` |

```bash
AICOMMIT_TIMEOUT=2m aicommit --commit
aicommit --timeout 120s --commit
```

## Testing

```bash
go test ./...
```

Tests use fakes and `httptest` — no real git repo or LLM server required.

## Project Structure

```
cmd/aicommit/main.go    # CLI entry point (Cobra)
internal/git/diff.go    # Diff retrieval (staged and all)
internal/git/commit.go  # Commit execution (staged and all)
internal/llm/client.go  # OpenAI-compatible LLM API client
internal/prompt/        # Prompt template construction
```