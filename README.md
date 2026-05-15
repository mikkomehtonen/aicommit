# aicommit

Generate Conventional Commit messages using a local LLM.

aicommit reads your git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout. By default it uses staged changes only; pass `--all` to include all changes.

## Prerequisites

- [Go 1.24+](https://go.dev/dl/)
- [LM Studio](https://lmstudio.ai/) running locally with a model loaded

## Install

```bash
go install ./cmd/aicommit
```

Or build directly:

```bash
go build -o aicommit ./cmd/aicommit
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

The app connects to LM Studio at `http://localhost:1234` using the model `qwen/qwen3.6-27b`. Both can be overridden with environment variables:

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

## Testing

```bash
go test ./...
```

Tests use fakes and `httptest` — no real git repo or LM Studio server required.

## Project Structure

```
cmd/aicommit/main.go    # CLI entry point (Cobra)
internal/git/diff.go    # Diff retrieval (staged and all)
internal/git/commit.go  # Commit execution (staged and all)
internal/llm/client.go  # LM Studio API client
internal/prompt/        # Prompt template construction
```