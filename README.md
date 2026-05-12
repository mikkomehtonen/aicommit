# aicommit

Generate Conventional Commit messages using a local LLM.

aicommit reads your staged git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout.

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

3. The generated commit message is printed to stdout. Use it however you like:

```bash
aicommit | git commit -F -
```

## Configuration

The app connects to LM Studio at `http://localhost:1234` using the model `qwen/qwen3.6-27b`. Override the model by setting the `AICOMMIT_MODEL` environment variable:

```bash
AICOMMIT_MODEL=my-model aicommit
```

## Project Structure

```
cmd/aicommit/main.go    # CLI entry point (Cobra)
internal/git/diff.go    # Staged diff retrieval
internal/llm/client.go  # LM Studio API client
internal/prompt/        # Prompt template construction
```