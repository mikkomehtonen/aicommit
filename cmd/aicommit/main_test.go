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
	err          error // return this error on next Commit/CommitAll call
}

func (f *fakeCommitter) Commit(msg string) error {
	if f.err != nil {
		err := f.err
		f.err = nil // one-shot error
		return err
	}
	f.committed = append(f.committed, msg)
	return nil
}

func (f *fakeCommitter) CommitAll(msg string) error {
	if f.err != nil {
		err := f.err
		f.err = nil // one-shot error
		return err
	}
	f.committedAll = append(f.committedAll, msg)
	return nil
}

type fakeGenerator struct {
	msgs    []string // responses returned in order
	index   int
	err     error    // return this error on next Generate call
	prompts []string // captured prompts passed to Generate
}

func (f *fakeGenerator) Generate(prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.err != nil {
		err := f.err
		f.err = nil // one-shot error
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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

	if err != errEmptyDiff {
		t.Errorf("expected errEmptyDiff, got %v", err)
	}
}

func TestRun_printMode(t *testing.T) {
	dp := &fakeDiffProvider{diff: "some diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: add something"}}
	var stdout, stderr bytes.Buffer

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Output should be the trimmed message with a trailing newline.
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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

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

	err := run(dp, mg, strings.NewReader(""), &stdout, &stderr, 0.1, 0.4)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "getting diff") {
		t.Errorf("error = %v, want error containing 'getting diff'", err)
	}
}

func TestInteractiveCommit_accept(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: new thing"}}
	c := &fakeCommitter{}
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("\n") // Enter = accept
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("r\na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("c\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 commits, got %d", len(c.committed))
	}
	if !strings.Contains(stderr.String(), "Cancelled") {
		t.Errorf("stderr = %q, want 'Cancelled'", stderr.String())
	}
}

func TestInteractiveCommit_emptyMessageRetries(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"  ", "feat: real message"}}
	c := &fakeCommitter{}
	stdin := strings.NewReader("a\na\n") // first accept hits empty, second succeeds
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(c.committed))
	}
	if c.committed[0] != "feat: real message" {
		t.Errorf("committed %q, want %q", c.committed[0], "feat: real message")
	}
	if !strings.Contains(stderr.String(), "empty") {
		t.Errorf("stderr should mention empty message, got %q", stderr.String())
	}
}

func TestInteractiveCommit_commitError(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{err: fmt.Errorf("git commit failed")}
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("x\na\n") // unknown then accept
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "Unknown choice") {
		t.Errorf("stderr should mention unknown choice, got %q", stderr.String())
	}
	if c.committed[0] != "feat: first" {
		t.Errorf("committed %q, want %q (unknown choice re-prompts same message)", c.committed[0], "feat: first")
	}
}

func TestInteractiveCommit_eof(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: something"}}
	c := &fakeCommitter{}
	stdin := strings.NewReader("") // immediate EOF
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\nfeat: edited message\na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\n\na\n") // edit with empty input, then accept
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\n   \na\n") // edit with whitespace-only input, then accept
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\nfeat: edited\nc\n") // edit, then cancel
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(c.committed) != 0 {
		t.Errorf("expected 0 commits, got %d", len(c.committed))
	}
	if !strings.Contains(stderr.String(), "Cancelled") {
		t.Errorf("stderr = %q, want 'Cancelled'", stderr.String())
	}
}

func TestInteractiveCommit_editThenRetry(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: original", "feat: regenerated"}}
	c := &fakeCommitter{}
	stdin := strings.NewReader("e\nfeat: edited\nr\na\n") // edit, then retry (regenerate), then accept
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\n  feat: trimmed  \na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("e\nfeat: first edit\ne\nfeat: final edit\na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.committed[0] != "feat: final edit" {
		t.Errorf("committed %q, want %q (last edit wins)", c.committed[0], "feat: final edit")
	}
}

// Verify that the realGit type satisfies the interfaces at compile time.
func TestInterfaceCompliance(t *testing.T) {
	var _ DiffProvider = (*realGit)(nil)
	var _ Committer = (*realGit)(nil)
	var _ MessageGenerator = (*llmClient)(nil)
}

// Wrapper to satisfy MessageGenerator for the real llm.Client.
type llmClient struct{}

func (llmClient) Generate(prompt string) (string, error) { return "", nil }

// Verify io.Reader/io.Writer are accepted where expected.
func TestRun_acceptsIOInterfaces(t *testing.T) {
	dp := &fakeDiffProvider{diff: "diff", err: nil}
	mg := &fakeGenerator{msgs: []string{"feat: test"}}
	var stdout, stderr bytes.Buffer

	// This just verifies the function signature accepts io.Reader/io.Writer.
	err := run(dp, mg, io.NopCloser(strings.NewReader("")), &stdout, &stderr, 0.1, 0.4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInteractiveCommit_allFlag_usesCommitAll(t *testing.T) {
	mg := &fakeGenerator{msgs: []string{"feat: all changes"}}
	c := &fakeCommitter{}
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, true, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, true, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("a\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

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
	stdin := strings.NewReader("r\na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 2 {
		t.Fatalf("expected 2 Generate calls, got %d", len(mg.prompts))
	}
	// First call should use the normal Build prompt.
	firstPrompt := prompt.Build("some diff")
	if mg.prompts[0] != firstPrompt {
		t.Errorf("first prompt = %q, want %q", mg.prompts[0], firstPrompt)
	}
	// Second call should use BuildRetry and include the rejected suggestion.
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
	stdin := strings.NewReader("r\nr\na\n")
	var stdout, stderr bytes.Buffer

	err := interactiveCommit("some diff", mg, c, false, stdin, &stdout, &stderr, 0.1, 0.4)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mg.prompts) != 3 {
		t.Fatalf("expected 3 Generate calls, got %d", len(mg.prompts))
	}
	// First call: normal Build.
	if mg.prompts[0] != prompt.Build("some diff") {
		t.Errorf("first prompt should be normal Build, got %q", mg.prompts[0])
	}
	// Second call: BuildRetry with one rejected suggestion.
	if !strings.Contains(mg.prompts[1], "feat: first") {
		t.Errorf("second prompt should contain first suggestion, got %q", mg.prompts[1])
	}
	// Third call: BuildRetry with two rejected suggestions.
	if !strings.Contains(mg.prompts[2], "feat: first") {
		t.Errorf("third prompt should contain first suggestion, got %q", mg.prompts[2])
	}
	if !strings.Contains(mg.prompts[2], "feat: second") {
		t.Errorf("third prompt should contain second suggestion, got %q", mg.prompts[2])
	}
}