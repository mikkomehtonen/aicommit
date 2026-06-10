package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"
)

type fakeDiffProvider struct {
	stagedDiff string
	stagedErr  error
	allDiff    string
	allErr     error
	headDiff   string
	headErr    error
	headMsg    string
	headMsgErr error
}

func (f *fakeDiffProvider) StagedDiff() (string, error) {
	return f.stagedDiff, f.stagedErr
}
func (f *fakeDiffProvider) AllDiff() (string, error) {
	return f.allDiff, f.allErr
}
func (f *fakeDiffProvider) HeadDiff() (string, error) {
	return f.headDiff, f.headErr
}
func (f *fakeDiffProvider) HeadMessage() (string, error) {
	return f.headMsg, f.headMsgErr
}

type fakeCommitter struct {
	commitErr       error
	commitAllErr    error
	rewordErr       error
	committedMsg    string
	committedAllMsg string
	rewordedMsg     string
}

func (f *fakeCommitter) Commit(msg string) error {
	f.committedMsg = msg
	return f.commitErr
}
func (f *fakeCommitter) CommitAll(msg string) error {
	f.committedAllMsg = msg
	return f.commitAllErr
}
func (f *fakeCommitter) RewordCommit(msg string) error {
	f.rewordedMsg = msg
	return f.rewordErr
}

type fakeGenerator struct {
	msgs        []string
	msgsIndex   int
	generateErr error
	temps       []float64
	prompts     []string
}

func (f *fakeGenerator) GenerateWithTemperature(ctx context.Context, prompt string, temperature float64) (string, error) {
	if f.generateErr != nil {
		return "", f.generateErr
	}
	if f.msgsIndex >= len(f.msgs) {
		return "", errors.New("no more messages")
	}
	msg := f.msgs[f.msgsIndex]
	f.msgsIndex++
	f.temps = append(f.temps, temperature)
	f.prompts = append(f.prompts, prompt)
	return msg, nil
}

func TestRun_stagedDiff_success(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedDiff: "some diff"},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		Temperature:  0.1,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen.msgsIndex != 1 {
		t.Fatalf("expected generator to be called once, got %d", gen.msgsIndex)
	}
	if gen.temps[0] != 0.1 {
		t.Errorf("temperature = %f, want 0.1", gen.temps[0])
	}
}

func TestRun_allDiff_success(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: all changes"}}
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{allDiff: "all diff output"},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		AllFlag:      true,
		Temperature:  0.1,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen.msgsIndex != 1 {
		t.Fatalf("expected generator to be called once, got %d", gen.msgsIndex)
	}
}

func TestRun_reword_success(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: reworded"}}
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{headDiff: "head diff", headMsg: "old msg"},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		RewordFlag:   true,
		Temperature:  0.1,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen.msgsIndex != 1 {
		t.Fatalf("expected generator to be called once, got %d", gen.msgsIndex)
	}
	if !strings.Contains(gen.prompts[0], "old msg") {
		t.Errorf("prompt should contain old message, got: %q", gen.prompts[0])
	}
}

func TestRun_emptyDiff_staged(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedDiff: "  \n\t  "},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	}
	if err := run(context.Background(), cfg); !errors.Is(err, errEmptyDiff) {
		t.Fatalf("expected errEmptyDiff, got: %v", err)
	}
}

func TestRun_emptyDiff_all(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{allDiff: "   "},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		AllFlag:      true,
	}
	if err := run(context.Background(), cfg); !errors.Is(err, errEmptyDiff) {
		t.Fatalf("expected errEmptyDiff, got: %v", err)
	}
}

func TestRun_emptyDiff_reword(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{headDiff: "\n"},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		RewordFlag:   true,
	}
	if err := run(context.Background(), cfg); !errors.Is(err, errEmptyDiff) {
		t.Fatalf("expected errEmptyDiff, got: %v", err)
	}
}

func TestRun_diffError(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedErr: errors.New("git error")},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_generateError(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedDiff: "diff"},
		Generator:    &fakeGenerator{generateErr: errors.New("llm error")},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_headMessageError(t *testing.T) {
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{headDiff: "diff", headMsgErr: errors.New("no commits")},
		Generator:    &fakeGenerator{msgs: []string{"msg"}},
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		RewordFlag:   true,
	}
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_nonInteractivePrints(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"  feat: add login  \n"}}
	var out strings.Builder
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedDiff: "diff"},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       &out,
		Stderr:       io.Discard,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != "feat: add login\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "feat: add login\n")
	}
}

func TestInteractiveCommit_accept(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: add feature" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: add feature")
	}
}

func TestInteractiveCommit_acceptDefault(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: add feature" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: add feature")
	}
}

func TestInteractiveCommit_allAccept(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: all changes"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\n")
	cfg := RunConfig{
		Generator: gen,
		Committer: cm,
		Stdin:     stdin,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
		AllFlag:   true,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", true, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedAllMsg != "feat: all changes" {
		t.Errorf("committed all message = %q, want %q", cm.committedAllMsg, "feat: all changes")
	}
}

func TestInteractiveCommit_rewordAccept(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: reworded"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\n")
	cfg := RunConfig{
		Generator:  gen,
		Committer:  cm,
		Stdin:      stdin,
		Stdout:     io.Discard,
		Stderr:     io.Discard,
		RewordFlag: true,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, true, "prompt", "old msg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.rewordedMsg != "feat: reworded" {
		t.Errorf("reworded message = %q, want %q", cm.rewordedMsg, "feat: reworded")
	}
}

func TestInteractiveCommit_edit(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("e\nfeat: edited message\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Committer: cm,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: edited message" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: edited message")
	}
}

func TestInteractiveCommit_editKeepsMessage(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("e\n\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Committer: cm,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: add feature" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: add feature")
	}
}

func TestInteractiveCommit_retry(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: first", "feat: second"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("r\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: second" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: second")
	}
	if gen.msgsIndex != 2 {
		t.Fatalf("expected generator called twice, got %d", gen.msgsIndex)
	}
	if len(gen.temps) != 2 || gen.temps[0] != 0.1 || gen.temps[1] != 0.4 {
		t.Errorf("temperatures = %v, want [0.1, 0.4]", gen.temps)
	}
}

func TestInteractiveCommit_cancel(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	stdin := strings.NewReader("c\n")
	var out, errOut strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    &errOut,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Cancelled") {
		t.Errorf("stderr = %q, want 'Cancelled'", errOut.String())
	}
}

func TestInteractiveCommit_unknownChoice(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("x\na\n")
	var out, errOut strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Committer: cm,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    &errOut,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Unknown choice") {
		t.Errorf("stderr = %q, want 'Unknown choice'", errOut.String())
	}
	if cm.committedMsg != "feat: add feature" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: add feature")
	}
}

func TestInteractiveCommit_emptyMessageRetry(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"", "feat: second"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\na\n")
	var out, errOut strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           &errOut,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: second" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: second")
	}
	if !strings.Contains(errOut.String(), "empty, retrying") {
		t.Errorf("stderr = %q, want 'empty, retrying'", errOut.String())
	}
	if gen.msgsIndex != 2 {
		t.Fatalf("expected generator called twice, got %d", gen.msgsIndex)
	}
}

func TestInteractiveCommit_emptyMessageMaxRetries(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"", "", "", ""}}
	stdin := strings.NewReader("a\na\na\na\n")
	var out, errOut strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           &errOut,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", "")
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}
	if !strings.Contains(err.Error(), "empty messages after 3 attempts") {
		t.Errorf("error = %q, want 'empty messages after 3 attempts'", err.Error())
	}
	if gen.msgsIndex != 4 {
		t.Fatalf("expected generator called 4 times, got %d", gen.msgsIndex)
	}
}

func TestInteractiveCommit_contextCancelGeneration(t *testing.T) {
	gen := &fakeGenerator{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdin := strings.NewReader("")
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(ctx, cfg, "diff", false, false, "prompt", ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestInteractiveCommit_contextCancelConfirm(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	ctx, cancel := context.WithCancel(context.Background())
	stdin := strings.NewReader("a\n")
	// Cancel before calling interactiveCommit to test ctx.Err() in confirm loop.
	cancel()
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(ctx, cfg, "diff", false, false, "prompt", ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestInteractiveCommit_eofChoice(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	stdin := strings.NewReader("")
	var out strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() == 0 {
		t.Error("expected stdout output")
	}
}

func TestInteractiveCommit_eofEdit(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	stdin := strings.NewReader("e")
	var out strings.Builder
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    &out,
		Stderr:    io.Discard,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInteractiveCommit_retryThenAccept(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: first", "feat: second"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("r\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: second" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: second")
	}
}

func TestInteractiveCommit_previousSuggestionsLimit(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"1", "2", "3", "4", "5", "6", "7"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("r\nr\nr\nr\nr\nr\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gen.msgsIndex != 7 {
		t.Fatalf("expected generator called 7 times, got %d", gen.msgsIndex)
	}
	// Verify that the 7th prompt was a retry prompt (should contain only last 5 suggestions)
	lastPrompt := gen.prompts[len(gen.prompts)-1]
	if !strings.Contains(lastPrompt, "3") {
		t.Errorf("last prompt should contain suggestion 3, got: %q", lastPrompt)
	}
	if !strings.Contains(lastPrompt, "4") {
		t.Errorf("last prompt should contain suggestion 4, got: %q", lastPrompt)
	}
	if strings.Contains(lastPrompt, "1") {
		t.Errorf("last prompt should not contain suggestion 1, got: %q", lastPrompt)
	}
}

func TestDiffForMode_staged(t *testing.T) {
	dp := &fakeDiffProvider{stagedDiff: "staged"}
	got, err := diffForMode(dp, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "staged" {
		t.Errorf("got %q, want %q", got, "staged")
	}
}

func TestDiffForMode_all(t *testing.T) {
	dp := &fakeDiffProvider{allDiff: "all"}
	got, err := diffForMode(dp, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "all" {
		t.Errorf("got %q, want %q", got, "all")
	}
}

func TestDiffForMode_reword(t *testing.T) {
	dp := &fakeDiffProvider{headDiff: "head"}
	got, err := diffForMode(dp, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "head" {
		t.Errorf("got %q, want %q", got, "head")
	}
}

func TestDiffForMode_stagedError(t *testing.T) {
	dp := &fakeDiffProvider{stagedErr: errors.New("git error")}
	_, err := diffForMode(dp, false, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDiffForMode_allError(t *testing.T) {
	dp := &fakeDiffProvider{allErr: errors.New("git error")}
	_, err := diffForMode(dp, true, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDiffForMode_rewordError(t *testing.T) {
	dp := &fakeDiffProvider{headErr: errors.New("git error")}
	_, err := diffForMode(dp, false, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveTemperature_explicit(t *testing.T) {
	got := resolveTemperature(0.7, true, 0.1)
	if got != 0.7 {
		t.Errorf("got %f, want 0.7", got)
	}
}

func TestResolveTemperature_default(t *testing.T) {
	got := resolveTemperature(0, false, 0.1)
	if got != 0.1 {
		t.Errorf("got %f, want 0.1", got)
	}
}

func TestResolveTemperature_negative(t *testing.T) {
	got := resolveTemperature(-0.5, true, 0.1)
	if got != 0 {
		t.Errorf("got %f, want 0", got)
	}
}

// scannerErrorReader returns a custom error on Read.
type scannerErrorReader struct{}

func (scannerErrorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("read error")
}

func TestInteractiveCommit_scannerErrorChoice(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	stdin := scannerErrorReader{}
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	}
	err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reading stdin") {
		t.Errorf("error = %q, want 'reading stdin'", err.Error())
	}
}

func TestInteractiveCommit_scannerErrorEdit(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	// The scanner will read "e\n" from this reader, then fail on the next read.
	stdin := io.MultiReader(strings.NewReader("e\n"), &errorReader{})
	cfg := RunConfig{
		Generator: gen,
		Stdin:     stdin,
		Stdout:    io.Discard,
		Stderr:    io.Discard,
	}
	err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "reading stdin") {
		t.Errorf("error = %q, want 'reading stdin'", err.Error())
	}
}

// errorReader always returns an error.
type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("read error")
}

func TestRun_reword_promptContainsBuildReword(t *testing.T) {
	gen := &fakeGenerator{msgs: []string{"feat: reworded"}}
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{headDiff: "diff", headMsg: "old message"},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		RewordFlag:   true,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gen.prompts[0], "old message") {
		t.Errorf("prompt should contain old message, got: %q", gen.prompts[0])
	}
}

func TestInteractiveCommit_emptyRetryMessageAddedToPrevious(t *testing.T) {
	// First generation is empty, second is valid.
	gen := &fakeGenerator{msgs: []string{"", "feat: second"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\na\n")
	var out, errOut strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           &errOut,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The second prompt should be a retry prompt (not initial prompt)
	if len(gen.prompts) < 2 {
		t.Fatal("expected 2 prompts")
	}
	if gen.prompts[1] == "prompt" {
		t.Error("second prompt should be a retry prompt, not the initial prompt")
	}
	if !strings.Contains(gen.prompts[1], "rejected") {
		t.Errorf("second prompt should contain 'rejected', got: %q", gen.prompts[1])
	}
}

func TestInteractiveCommit_editWithRetryPrompt(t *testing.T) {
	// First: generate "1", user retries, then edits and accepts.
	gen := &fakeGenerator{msgs: []string{"1", "2"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("r\ne\nfeat: edited\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "feat: edited" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "feat: edited")
	}
	if gen.msgsIndex != 2 {
		t.Fatalf("expected generator called twice, got %d", gen.msgsIndex)
	}
	if gen.temps[1] != 0.4 {
		t.Errorf("retry temperature = %f, want 0.4", gen.temps[1])
	}
}

func TestInteractiveCommit_emptyRetryThenRetry(t *testing.T) {
	// First: empty, auto-retry. Second: "1", user retries. Third: "2", accept.
	gen := &fakeGenerator{msgs: []string{"", "1", "2"}}
	cm := &fakeCommitter{}
	stdin := strings.NewReader("a\nr\na\n")
	var out strings.Builder
	cfg := RunConfig{
		Generator:        gen,
		Committer:        cm,
		Stdin:            stdin,
		Stdout:           &out,
		Stderr:           io.Discard,
		Temperature:      0.1,
		RetryTemperature: 0.4,
	}
	if err := interactiveCommit(context.Background(), cfg, "diff", false, false, "prompt", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cm.committedMsg != "2" {
		t.Errorf("committed message = %q, want %q", cm.committedMsg, "2")
	}
	if gen.msgsIndex != 3 {
		t.Fatalf("expected generator called 3 times, got %d", gen.msgsIndex)
	}
	// All generations after first should use retry temp.
	if gen.temps[0] != 0.1 {
		t.Errorf("first temp = %f, want 0.1", gen.temps[0])
	}
	if gen.temps[1] != 0.4 {
		t.Errorf("second temp = %f, want 0.4", gen.temps[1])
	}
	if gen.temps[2] != 0.4 {
		t.Errorf("third temp = %f, want 0.4", gen.temps[2])
	}
}

func TestTruncateDiff_underLimit(t *testing.T) {
	var buf strings.Builder
	diff := "small diff"
	got := truncateDiff(diff, &buf)
	if got != diff {
		t.Errorf("got %q, want %q", got, diff)
	}
	if buf.String() != "" {
		t.Errorf("unexpected warning: %q", buf.String())
	}
}

func TestTruncateDiff_overLimit(t *testing.T) {
	var buf strings.Builder
	diff := strings.Repeat("a", maxDiffSize+10)
	got := truncateDiff(diff, &buf)
	if len(got) != maxDiffSize+len("\n... (truncated)") {
		t.Errorf("truncated length = %d, want %d", len(got), maxDiffSize+len("\n... (truncated)"))
	}
	if !strings.HasSuffix(got, "\n... (truncated)") {
		t.Errorf("expected truncation suffix, got: %q", got)
	}
	if !strings.Contains(buf.String(), "warning: diff exceeds") {
		t.Errorf("expected warning, got: %q", buf.String())
	}
}

func TestTruncateDiff_utf8Boundary(t *testing.T) {
	var buf strings.Builder
	// 3-byte UTF-8 char "世" repeated to exceed maxDiffSize boundary
	diff := strings.Repeat("世", maxDiffSize/3+10)
	got := truncateDiff(diff, &buf)
	if !utf8.ValidString(got) {
		t.Errorf("truncated result contains invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "\n... (truncated)") {
		t.Errorf("expected truncation suffix, got: %q", got)
	}
}

func TestRun_largeDiffTruncated(t *testing.T) {
	largeDiff := strings.Repeat("a", maxDiffSize+10)
	gen := &fakeGenerator{msgs: []string{"feat: add feature"}}
	var errOut strings.Builder
	cfg := RunConfig{
		DiffProvider: &fakeDiffProvider{stagedDiff: largeDiff},
		Generator:    gen,
		Stdin:        strings.NewReader(""),
		Stdout:       io.Discard,
		Stderr:       &errOut,
		Temperature:  0.1,
	}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "warning: diff exceeds") {
		t.Errorf("expected warning in stderr, got: %q", errOut.String())
	}
	if !strings.Contains(gen.prompts[0], "\n... (truncated)") {
		t.Errorf("prompt should contain truncation suffix")
	}
}

