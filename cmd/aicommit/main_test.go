package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"aicommit/internal/prompt"
)

// --- Fakes ---

type fakeDiffProvider struct {
	diff string
	err  error
}

func (f *fakeDiffProvider) StagedDiff() (string, error) { return f.diff, f.err }

func (f *fakeDiffProvider) AllDiff() (string, error) { return f.diff, f.err }

type fakeCommitter struct {
	committed    []string
	committedAll []string
	err          error
}

func (f *fakeCommitter) Commit(msg string) error {
	if f.err != nil {
		err := f.err
		f.err = nil
		return err
	}
	f.committed = append(f.committed, msg)
	return nil
}

func (f *fakeCommitter) CommitAll(msg string) error {
	if f.err != nil {
		err := f.err
		f.err = nil
		return err
	}
	f.committedAll = append(f.committedAll, msg)
	return nil
}

type fakeGenerator struct {
	msgs    []string
	index   int
	err     error
	prompts []string
}

func (f *fakeGenerator) Generate(prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		err := f.err
		f.err = nil
		return "", err
	}
	if f.index < len(f.msgs) {
		msg := f.msgs[f.index]
		f.index++
		return msg, nil
	}
	return "", fmt.Errorf("no more fake responses")
}

// --- Tests ---

func TestRun_emptyDiff(t *testing.T) {
	dp := &fakeDiffProvider{diff: "", err: nil}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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

	err := run(RunConfig{
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

	err := run(RunConfig{
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
	mg := &fakeGenerator{err: fmt.Errorf("LLM is down")}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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

	err := run(RunConfig{
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
	allFlag = true
	defer func() { allFlag = false }()

	dp := &fakeDiffProvider{diff: "all changes diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: all changes"}}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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
	if stdout.String() != "feat: all changes\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "feat: all changes\n")
	}
}

func TestRun_allFlag_emptyDiff(t *testing.T) {
	allFlag = true
	defer func() { allFlag = false }()

	dp := &fakeDiffProvider{diff: "", err: nil}
	mg := &fakeGenerator{msgs: []string{"should not be called"}}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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
	if !strings.Contains(stderr.String(), "No changes found") {
		t.Errorf("stderr = %q, want 'No changes found'", stderr.String())
	}
}

func TestRun_allFlag_diffError(t *testing.T) {
	allFlag = true
	defer func() { allFlag = false }()

	dp := &fakeDiffProvider{err: fmt.Errorf("git not found")}
	mg := &fakeGenerator{msgs: []string{"irrelevant"}}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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
	if !strings.Contains(err.Error(), "getting diff") {
		t.Errorf("error = %v, want error containing 'getting diff'", err)
	}
}

func makeIC(mg *fakeGenerator, c *fakeCommitter, stdin string, all bool) RunConfig {
	var stdout, stderr bytes.Buffer
	return RunConfig{
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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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
	c := &fakeCommitter{err: fmt.Errorf("git commit failed")}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(cfg, "some diff", false)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "committing") {
		t.Errorf("error = %v, want error containing 'committing'", err)
	}
}

func TestInteractiveCommit_generateError(t *testing.T) {
	mg := &fakeGenerator{err: fmt.Errorf("LLM error")}
	c := &fakeCommitter{}
	cfg := makeIC(mg, c, "a\n", false)

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.committed[0] != "feat: final edit" {
		t.Errorf("committed %q, want %q (last edit wins)", c.committed[0], "feat: final edit")
	}
}

func TestInterfaceCompliance(t *testing.T) {
	var _ DiffProvider = (*realGit)(nil)
	var _ Committer = (*realGit)(nil)
	var _ MessageGenerator = (*llmClient)(nil)
}

type llmClient struct{}

func (llmClient) Generate(prompt string) (string, error) { return "", nil }

func TestRun_acceptsIOInterfaces(t *testing.T) {
	dp := &fakeDiffProvider{diff: "diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: test"}}
	var stdout, stderr bytes.Buffer

	err := run(RunConfig{
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

	err := interactiveCommit(cfg, "some diff", true)

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
	c := &fakeCommitter{err: fmt.Errorf("git commit -a failed")}
	cfg := makeIC(mg, c, "a\n", true)

	err := interactiveCommit(cfg, "some diff", true)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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

	err := interactiveCommit(cfg, "some diff", false)

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
