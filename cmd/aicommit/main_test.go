package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"aicommit/internal/git"
	"aicommit/internal/llm"
	"aicommit/internal/prompt"
)

// --- Fakes ---

type fakeDiffProvider struct {
	diff        string
	err         error
	headDiff    string
	headDiffErr error
	headMsg     string
	headMsgErr  error
}

func (f *fakeDiffProvider) StagedDiff() (string, error) { return f.diff, f.err }

func (f *fakeDiffProvider) AllDiff() (string, error) { return f.diff, f.err }

func (f *fakeDiffProvider) HeadDiff() (string, error) { return f.headDiff, f.headDiffErr }

func (f *fakeDiffProvider) HeadMessage() (string, error) { return f.headMsg, f.headMsgErr }

type fakeCommitter struct {
	committed    []string
	committedAll []string
	reworded     []string
	errs         []error
	errIndex     int
}

func (f *fakeCommitter) Commit(msg string) error {
	if f.errIndex < len(f.errs) {
		err := f.errs[f.errIndex]
		f.errIndex++
		return err
	}
	f.committed = append(f.committed, msg)
	return nil
}

func (f *fakeCommitter) CommitAll(msg string) error {
	if f.errIndex < len(f.errs) {
		err := f.errs[f.errIndex]
		f.errIndex++
		return err
	}
	f.committedAll = append(f.committedAll, msg)
	return nil
}

func (f *fakeCommitter) RewordCommit(msg string) error {
	if f.errIndex < len(f.errs) {
		err := f.errs[f.errIndex]
		f.errIndex++
		return err
	}
	f.reworded = append(f.reworded, msg)
	return nil
}

type fakeGenerator struct {
	msgs    []string
	index   int
	errs    []error
	errIdx  int
	prompts []string
}

func (f *fakeGenerator) Generate(ctx context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.errIdx < len(f.errs) {
		err := f.errs[f.errIdx]
		f.errIdx++
		return "", err
	}
	if f.index < len(f.msgs) {
		msg := f.msgs[f.index]
		f.index++
		return msg, nil
	}
	return "", fmt.Errorf("no more fake responses")
}

func (f *fakeGenerator) GenerateWithTemperature(ctx context.Context, prompt string, temperature float64) (string, error) {
	return f.Generate(ctx, prompt)
}

// --- Tests ---

func TestRun_emptyDiff(t *testing.T) {
	dp := &fakeDiffProvider{diff: "", err: nil}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
	if stdout.String() != "" {
		t.Errorf("expected no stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "No staged changes found") {
		t.Errorf("expected hint on stderr, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--all") {
		t.Errorf("expected --all hint on stderr, got %q", stderr.String())
	}
}

func TestRun_whitespaceOnlyDiff(t *testing.T) {
	dp := &fakeDiffProvider{diff: "   \n\t  ", err: nil}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
}

func TestRun_printMode(t *testing.T) {
	dp := &fakeDiffProvider{diff: "some diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: add something"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	want := "feat: add something\n"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_generateError(t *testing.T) {
	dp := &fakeDiffProvider{diff: "some diff", err: nil}
	mg := &fakeGenerator{errs: []error{fmt.Errorf("LLM is down")}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "generating commit message") {
		t.Errorf("error = %v, want error containing 'generating commit message'", err)
	}
}

func TestRun_diffError(t *testing.T) {
	dp := &fakeDiffProvider{err: fmt.Errorf("git not found")}
	mg := &fakeGenerator{msgs: []string{"irrelevant"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting staged diff") {
		t.Errorf("error = %v, want error containing 'getting staged diff'", err)
	}
}

func TestRun_allFlag_usesAllDiff(t *testing.T) {
	dp := &fakeDiffProvider{diff: "all changes diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: all changes"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		AllFlag:          true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stdout.String() != "feat: all changes\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "feat: all changes\n")
	}
}

func TestRun_allFlag_emptyDiff(t *testing.T) {
	dp := &fakeDiffProvider{diff: "", err: nil}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		AllFlag:          true,
	})

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
	if !strings.Contains(stderr.String(), "No changes found") {
		t.Errorf("stderr = %q, want 'No changes found'", stderr.String())
	}
}

func TestRun_allFlag_diffError(t *testing.T) {
	dp := &fakeDiffProvider{err: fmt.Errorf("git not found")}
	mg := &fakeGenerator{msgs: []string{"irrelevant"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		AllFlag:          true,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting diff") {
		t.Errorf("error = %v, want error containing 'getting diff'", err)
	}
}

func makeIC(mg *fakeGenerator, c *fakeCommitter, stdin string, all bool) RunConfig {
	var stdout, stderr bytes.Buffer
	return RunConfig{
		DiffProvider:     &fakeDiffProvider{diff: "test diff", headMsg: "old message"},
		Generator:        mg,
		Committer:        c,
		Stdin:            strings.NewReader(stdin),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
}

func TestInteractiveCommit_accept(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: new thing"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: new thing" {
		t.Errorf("committed %q, want %q", c.committed[0], "feat: new thing")
	}
}

func TestInteractiveCommit_acceptWithEnter(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"fix: bug"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
}

func TestInteractiveCommit_retry(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"bad message", "good message"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "r\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "good message" {
		t.Errorf("committed %q, want %q", c.committed[0], "good message")
	}
}

func TestInteractiveCommit_cancel(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "c\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 commits, got %d", len(c.committed))
	}
	if !strings.Contains(cfg.Stderr.(*bytes.Buffer).String(), "Cancelled") {
		t.Errorf("stderr = %q, want 'Cancelled'", cfg.Stderr.(*bytes.Buffer).String())
	}
}

func TestInteractiveCommit_emptyMessageRetries(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"  ", "feat: real message"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: real message" {
		t.Errorf("committed %q, want %q", c.committed[0], "feat: real message")
	}
	if !strings.Contains(cfg.Stderr.(*bytes.Buffer).String(), "empty") {
		t.Errorf("stderr should mention empty message, got %q", cfg.Stderr.(*bytes.Buffer).String())
	}
}

func TestInteractiveCommit_commitError(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{errs: []error{fmt.Errorf("git commit failed")}}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "committing") {
		t.Errorf("error = %v, want error containing 'committing'", err)
	}
}

func TestInteractiveCommit_generateError(t *testing.T) {
	mg := &fakeGenerator{errs: []error{fmt.Errorf("LLM error")}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "generating commit message") {
		t.Errorf("error = %v, want error containing 'generating commit message'", err)
	}
}

func TestInteractiveCommit_unknownChoice(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: first"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "x\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(cfg.Stderr.(*bytes.Buffer).String(), "Unknown choice") {
		t.Errorf("stderr should mention unknown choice, got %q", cfg.Stderr.(*bytes.Buffer).String())
	}
	if c.committed[0] != "feat: first" {
		t.Errorf("committed %q, want %q (unknown choice re-prompts same message)", c.committed[0], "feat: first")
	}
}

func TestInteractiveCommit_eof(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 commits on EOF, got %d", len(c.committed))
	}
}

func TestInteractiveCommit_editThenAccept(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\nfeat: edited message\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: edited message" {
		t.Errorf("committed %q, want %q", c.committed[0], "feat: edited message")
	}
}

func TestInteractiveCommit_editEmptyKeepsOriginal(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\n\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: original" {
		t.Errorf("committed %q, want %q (original kept on empty edit)", c.committed[0], "feat: original")
	}
}

func TestInteractiveCommit_editWhitespaceOnlyKeepsOriginal(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\n   \na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.committed[0] != "feat: original" {
		t.Errorf("committed %q, want %q (original kept on whitespace-only edit)", c.committed[0], "feat: original")
	}
}

func TestInteractiveCommit_editThenCancel(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\nfeat: edited\nc\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 commits, got %d", len(c.committed))
	}
	if !strings.Contains(cfg.Stderr.(*bytes.Buffer).String(), "Cancelled") {
		t.Errorf("stderr = %q, want 'Cancelled'", cfg.Stderr.(*bytes.Buffer).String())
	}
}

func TestInteractiveCommit_editThenRetry(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original", "feat: regenerated"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\nfeat: edited\nr\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: regenerated" {
		t.Errorf("committed %q, want %q (regenerated after retry)", c.committed[0], "feat: regenerated")
	}
}

func TestInteractiveCommit_editTrimsWhitespace(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\n  feat: trimmed  \na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.committed[0] != "feat: trimmed" {
		t.Errorf("committed %q, want %q (trimmed edit)", c.committed[0], "feat: trimmed")
	}
}

func TestInteractiveCommit_multipleEdits(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "e\nfeat: first edit\ne\nfeat: final edit\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.committed[0] != "feat: final edit" {
		t.Errorf("committed %q, want %q (last edit wins)", c.committed[0], "feat: final edit")
	}
}

func TestGenerateWithTemperature(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: with temp"}}
	got, err := mg.GenerateWithTemperature(context.Background(), "test prompt", 0.7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "feat: with temp" {
		t.Errorf("got %q, want %q", got, "feat: with temp")
	}
	if len(mg.prompts) != 1 {
		t.Errorf("expected 1 call, got %d", len(mg.prompts))
	}
}

func TestResolveTemperature(t *testing.T) {
	tests := []struct {
		name     string
		flag     float64
		changed  bool
		def      float64
		expected float64
	}{
		{"flag changed", 0.7, true, 0.1, 0.7},
		{"flag not changed", 0, false, 0.1, 0.1},
		{"flag zero but changed", 0, true, 0.5, 0},
		{"flag not changed with non-zero default", 0, false, 0.5, 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTemperature(tt.flag, tt.changed, tt.def)
			if got != tt.expected {
				t.Errorf("resolveTemperature(%v, %v, %v) = %v, want %v", tt.flag, tt.changed, tt.def, got, tt.expected)
			}
		})
	}
}

func TestResolveTemperature_negativeClamped(t *testing.T) {
	// resolveTemperature clamps negative values to 0.
	got := resolveTemperature(-0.5, true, 0.1)
	if got != 0 {
		t.Errorf("resolveTemperature(-0.5, true, 0.1) = %v, want 0", got)
	}
}

func TestInterfaceCompliance(t *testing.T) {
	var _ DiffProvider = (*git.Git)(nil)
	var _ Committer = (*git.Git)(nil)
	var _ MessageGeneratorWithTemperature = (*llm.Client)(nil)
}

func TestRun_acceptsIOInterfaces(t *testing.T) {
	dp := &fakeDiffProvider{diff: "diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: test"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            io.NopCloser(strings.NewReader("")),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInteractiveCommit_allFlag_usesCommitAll(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: all changes"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", true)

	err := interactiveCommit(context.Background(), cfg, "some diff", true, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committedAll) != 1 {
		t.Fatalf("expected 1 CommitAll call, got %d", len(c.committedAll))
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 Commit calls, got %d", len(c.committed))
	}
	if c.committedAll[0] != "feat: all changes" {
		t.Errorf("committedAll %q, want %q", c.committedAll[0], "feat: all changes")
	}
}

func TestInteractiveCommit_allFlag_commitAllError(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{errs: []error{fmt.Errorf("git commit -a failed")}}
	cfg := makeIC(mg, c, "a\n", true)

	err := interactiveCommit(context.Background(), cfg, "some diff", true, false, prompt.Build("some diff"), "")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "committing") {
		t.Errorf("error = %v, want error containing 'committing'", err)
	}
}

func TestInteractiveCommit_noAllFlag_usesCommit(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: staged changes"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 Commit call, got %d", len(c.committed))
	}
	if len(c.committedAll) != 0 {
		t.Errorf("expected 0 CommitAll calls, got %d", len(c.committedAll))
	}
}

func TestInteractiveCommit_retryUsesBuildRetry(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: first attempt", "feat: second attempt"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "r\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 2 {
		t.Fatalf("expected 2 Generate calls, got %d", len(mg.prompts))
	}
	firstPrompt := prompt.Build("some diff")
	if mg.prompts[0] != firstPrompt {
		t.Errorf("first prompt = %q, want %q", mg.prompts[0], firstPrompt)
	}
	if !strings.Contains(mg.prompts[1], "feat: first attempt") {
		t.Errorf("retry prompt should contain rejected suggestion, got %q", mg.prompts[1])
	}
	if !strings.Contains(mg.prompts[1], "rejected") {
		t.Errorf("retry prompt should mention 'rejected', got %q", mg.prompts[1])
	}
	if !strings.Contains(mg.prompts[1], "different") {
		t.Errorf("retry prompt should mention 'different', got %q", mg.prompts[1])
	}
}

func TestInteractiveCommit_multipleRetriesAccumulate(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: first", "feat: second", "feat: third"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "r\nr\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 3 {
		t.Fatalf("expected 3 Generate calls, got %d", len(mg.prompts))
	}
	if mg.prompts[0] != prompt.Build("some diff") {
		t.Errorf("first prompt should be normal Build, got %q", mg.prompts[0])
	}
	if !strings.Contains(mg.prompts[1], "feat: first") {
		t.Errorf("second prompt should contain first suggestion, got %q", mg.prompts[1])
	}
	if !strings.Contains(mg.prompts[2], "feat: first") {
		t.Errorf("third prompt should contain first suggestion, got %q", mg.prompts[2])
	}
	if !strings.Contains(mg.prompts[2], "feat: second") {
		t.Errorf("third prompt should contain second suggestion, got %q", mg.prompts[2])
	}
}

func TestInteractiveCommit_retryCapsAt5(t *testing.T) {
	// Generate 9 messages: initial + 8 retries.
	msgs := []string{
		"feat: msg0",
		"feat: msg1",
		"feat: msg2",
		"feat: msg3",
		"feat: msg4",
		"feat: msg5",
		"feat: msg6",
		"feat: msg7",
		"feat: msg8",
	}
	mg := &fakeGenerator{msgs: msgs}
	c := &fakeCommitter{}
	// 8 retries then accept
	cfg := makeIC(mg, c, "r\nr\nr\nr\nr\nr\nr\nr\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, false, prompt.Build("some diff"), "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 9 {
		t.Fatalf("expected 9 Generate calls, got %d", len(mg.prompts))
	}
	// After 5 retries (at prompt index 5), the cap is triggered on append of msg5.
	// So prompt[6] is the first prompt built with the trimmed list [msg1..msg5].
	// It should NOT contain msg0 (oldest, capped out), but SHOULD contain msg1..msg5.
	if strings.Contains(mg.prompts[6], "feat: msg0") {
		t.Errorf("prompt[6] should not contain msg0 (capped)")
	}
	if !strings.Contains(mg.prompts[6], "feat: msg1") {
		t.Errorf("prompt[6] should contain msg1 (within cap)")
	}
	if !strings.Contains(mg.prompts[6], "feat: msg2") {
		t.Errorf("prompt[6] should contain msg2 (within cap)")
	}
	if !strings.Contains(mg.prompts[6], "feat: msg3") {
		t.Errorf("prompt[6] should contain msg3 (within cap)")
	}
	if !strings.Contains(mg.prompts[6], "feat: msg4") {
		t.Errorf("prompt[6] should contain msg4 (within cap)")
	}
	if !strings.Contains(mg.prompts[6], "feat: msg5") {
		t.Errorf("prompt[6] should contain msg5 (within cap)")
	}
	// msg6 is generated by prompt[6] and added after; it should not appear in prompt[6]
	if strings.Contains(mg.prompts[6], "feat: msg6") {
		t.Errorf("prompt[6] should not contain future msg6")
	}
	// Prompt[8] (last retry before accept) should have [msg3..msg7]
	if !strings.Contains(mg.prompts[8], "feat: msg3") {
		t.Errorf("prompt[8] should contain msg3")
	}
	if !strings.Contains(mg.prompts[8], "feat: msg7") {
		t.Errorf("prompt[8] should contain msg7")
	}
	if strings.Contains(mg.prompts[8], "feat: msg0") {
		t.Errorf("prompt[8] should not contain msg0 (long capped)")
	}
	if strings.Contains(mg.prompts[8], "feat: msg2") {
		t.Errorf("prompt[8] should not contain msg2 (capped out by prompt[8])")
	}
}

// --- Integration Tests ---

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, out)
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "aicommit-integ-*")
	if err != nil {
		t.Fatal(err)
	}
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
		_ = os.RemoveAll(dir)
	})
	return dir
}

// --- Reword Tests ---

func TestRun_reword_printMode(t *testing.T) {
	dp := &fakeDiffProvider{headDiff: "some head diff", headMsg: "old message"}
	mg := &fakeGenerator{msgs: []string{"feat: reworded message"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		RewordFlag:       true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := stdout.String()
	want := "feat: reworded message\n"
	if got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRun_reword_usesBuildReword(t *testing.T) {
	dp := &fakeDiffProvider{headDiff: "head diff", headMsg: "old message"}
	mg := &fakeGenerator{msgs: []string{"feat: reworded"}}
	var stdout bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		RewordFlag:       true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 1 {
		t.Fatalf("expected 1 Generate call, got %d", len(mg.prompts))
	}
	if !strings.Contains(mg.prompts[0], "old message") {
		t.Errorf("reword prompt should contain current message, got %q", mg.prompts[0])
	}
	if !strings.Contains(mg.prompts[0], "head diff") {
		t.Errorf("reword prompt should contain diff, got %q", mg.prompts[0])
	}
}

func TestRun_reword_emptyHeadDiff(t *testing.T) {
	dp := &fakeDiffProvider{headDiff: "", headMsg: "old message"}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		RewordFlag:       true,
	})

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
	if !strings.Contains(stderr.String(), "No changes in the current commit") {
		t.Errorf("stderr = %q, want 'No changes in the current commit'", stderr.String())
	}
}

func TestRun_reword_noCommits(t *testing.T) {
	dp := &fakeDiffProvider{headDiffErr: fmt.Errorf("no commits exist in this repository")}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		RewordFlag:       true,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no commits exist") {
		t.Errorf("error = %v, want error containing 'no commits exist'", err)
	}
}

func TestRun_reword_noMessage(t *testing.T) {
	dp := &fakeDiffProvider{headDiff: "some diff", headMsgErr: fmt.Errorf("no commits exist")}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(context.Background(), RunConfig{
		DiffProvider:     dp,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		RewordFlag:       true,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting current commit message") {
		t.Errorf("error = %v, want error containing 'getting current commit message'", err)
	}
}

func TestInteractiveCommit_reword_usesRewordCommit(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: reworded"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, true, prompt.BuildReword("some diff", "old message"), "old message")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.reworded) != 1 {
		t.Fatalf("expected 1 RewordCommit call, got %d", len(c.reworded))
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 Commit calls, got %d", len(c.committed))
	}
	if c.reworded[0] != "feat: reworded" {
		t.Errorf("reworded %q, want %q", c.reworded[0], "feat: reworded")
	}
}

func TestInteractiveCommit_reword_retryUsesBuildRewordRetry(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: first", "feat: second"}}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "r\na\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, true, prompt.BuildReword("some diff", "old message"), "old message")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 2 {
		t.Fatalf("expected 2 Generate calls, got %d", len(mg.prompts))
	}
	if !strings.Contains(mg.prompts[0], "old message") {
		t.Errorf("first reword prompt should contain current message, got %q", mg.prompts[0])
	}
	if !strings.Contains(mg.prompts[1], "feat: first") {
		t.Errorf("retry prompt should contain rejected suggestion, got %q", mg.prompts[1])
	}
	if !strings.Contains(mg.prompts[1], "old message") {
		t.Errorf("retry prompt should still contain current message, got %q", mg.prompts[1])
	}
	if !strings.Contains(mg.prompts[1], "rejected") {
		t.Errorf("retry prompt should mention 'rejected', got %q", mg.prompts[1])
	}
}

func TestInteractiveCommit_reword_commitError(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: reworded"}}
	c := &fakeCommitter{errs: []error{fmt.Errorf("git commit --amend failed")}}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(context.Background(), cfg, "some diff", false, true, prompt.BuildReword("some diff", "old message"), "old message")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "amending commit") {
		t.Errorf("error = %v, want error containing 'amending commit'", err)
	}
}

// --- Integration Tests ---

func TestIntegration_emptyStagedDiff(t *testing.T) {
	_ = chdirTemp(t)
	runGit(t, ".", "init", "-b", "main")

	g := git.New()
	var stdout, stderr bytes.Buffer
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	err := run(context.Background(), RunConfig{
		DiffProvider:     g,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
}

func TestIntegration_stagedChangesPrintMode(t *testing.T) {
	_ = chdirTemp(t)
	runGit(t, ".", "init", "-b", "main")
	if err := os.WriteFile("hello.txt", []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ".", "add", "hello.txt")

	g := git.New()
	staged, err := g.StagedDiff()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(staged, "hello") {
		t.Fatalf("staged diff should contain file content, got: %s", staged)
	}

	var stdout bytes.Buffer
	mg := &fakeGenerator{msgs: []string{"feat: add hello.txt"}}
	err = run(context.Background(), RunConfig{
		DiffProvider:     g,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "feat: add hello.txt") {
		t.Errorf("stdout = %q, want 'feat: add hello.txt'", stdout.String())
	}
}

func TestIntegration_allFlagNoHEAD(t *testing.T) {
	_ = chdirTemp(t)
	runGit(t, ".", "init", "-b", "main")
	if err := os.WriteFile("new.txt", []byte("new file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ".", "add", "new.txt")

	g := git.New()
	diff, err := g.AllDiff()
	if err != nil {
		t.Fatalf("AllDiff failed: %v", err)
	}
	if !strings.Contains(diff, "new file") {
		t.Fatalf("AllDiff fallback should contain file content, got: %s", diff)
	}

	var stdout bytes.Buffer
	mg := &fakeGenerator{msgs: []string{"feat: add new.txt"}}
	err = run(context.Background(), RunConfig{
		DiffProvider:     g,
		Generator:        mg,
		Committer:        &fakeCommitter{},
		Stdin:            strings.NewReader(""),
		Stdout:           &stdout,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		AllFlag:          true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "feat: add new.txt") {
		t.Errorf("stdout = %q, want 'feat: add new.txt'", stdout.String())
	}
}

func TestIntegration_interactiveCommitRealGit(t *testing.T) {
	_ = chdirTemp(t)
	runGit(t, ".", "init", "-b", "main")
	runGit(t, ".", "config", "user.email", "test@test.com")
	runGit(t, ".", "config", "user.name", "Test")
	if err := os.WriteFile("initial.txt", []byte("initial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ".", "add", "initial.txt")
	runGit(t, ".", "commit", "-m", "initial")

	if err := os.WriteFile("change.txt", []byte("change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, ".", "add", "change.txt")

	g := git.New()
	c := &fakeCommitter{}
	mg := &fakeGenerator{msgs: []string{"feat: add change.txt"}}
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), RunConfig{
		DiffProvider:     g,
		Generator:        mg,
		Committer:        c,
		Stdin:            strings.NewReader("a\n"),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Temperature:      0.1,
		RetryTemperature: 0.4,
		CommitFlag:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: add change.txt" {
		t.Errorf("committed %q, want %q", c.committed[0], "feat: add change.txt")
	}
}
