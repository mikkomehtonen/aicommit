package git

import (
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type fakeExecutor struct {
	fn func(cmd *exec.Cmd) ([]byte, error)
}

func (f *fakeExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return f.fn(cmd)
}

func (f *fakeExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return f.fn(cmd)
}

type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }
func (e *fakeExitError) ExitCode() int { return e.code }

func TestStagedDiff(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "diff", "--staged"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		return []byte("staged diff output"), nil
	}}
	g := &Git{Exec: fakeExec}

	out, err := g.StagedDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "staged diff output" {
		t.Errorf("got %q, want %q", out, "staged diff output")
	}
}

func TestAllDiff(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "diff", "HEAD"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		return []byte("all diff output"), nil
	}}
	g := &Git{Exec: fakeExec}

	out, err := g.AllDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "all diff output" {
		t.Errorf("got %q, want %q", out, "all diff output")
	}
}

func TestAllDiff_noHEAD_fallback(t *testing.T) {
	calls := 0
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("fatal: bad revision 'HEAD'"), &fakeExitError{code: 128}
		} else if calls == 2 {
			wantArgs := []string{"git", "diff", "--staged"}
			if !reflect.DeepEqual(cmd.Args, wantArgs) {
				return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
			}
			return []byte("staged diff output"), nil
		} else {
			wantArgs := []string{"git", "diff"}
			if !reflect.DeepEqual(cmd.Args, wantArgs) {
				return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
			}
			return []byte("unstaged diff output"), nil
		}
	}}
	g := &Git{Exec: fakeExec}

	out, err := g.AllDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "staged diff output\nunstaged diff output" {
		t.Errorf("got %q, want %q", out, "staged diff output\nunstaged diff output")
	}
}

func TestAllDiff_noHEAD_emptyFallback(t *testing.T) {
	calls := 0
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte("fatal: bad revision 'HEAD'"), &fakeExitError{code: 128}
		}
		return []byte(""), nil
	}}
	g := &Git{Exec: fakeExec}

	out, err := g.AllDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("got %q, want empty string", out)
	}
}

func TestAllDiff_exitCode128_otherError(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("fatal: not a git repository"), &fakeExitError{code: 128}
	}}
	g := &Git{Exec: fakeExec}

	_, err := g.AllDiff()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("error = %v, want error containing stderr text", err)
	}
}

func TestUnstagedDiff(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "diff"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		return []byte("unstaged diff output"), nil
	}}
	g := &Git{Exec: fakeExec}

	out, err := g.UnstagedDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "unstaged diff output" {
		t.Errorf("got %q, want %q", out, "unstaged diff output")
	}
}

func TestStagedDiff_error(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		return nil, fmt.Errorf("exit status 128")
	}}
	g := &Git{Exec: fakeExec}

	_, err := g.StagedDiff()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "running git diff --staged") {
		t.Errorf("error = %v, want error containing command text", err)
	}
}

func TestCommit(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "commit", "-F", "-"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		if cmd.Stdin != nil {
			stdinBytes, _ := io.ReadAll(cmd.Stdin)
			if string(stdinBytes) != "feat: add login\n\nAdds OAuth2 flow with PKCE." {
				return nil, fmt.Errorf("unexpected stdin: %q", string(stdinBytes))
			}
		} else {
			return nil, fmt.Errorf("expected stdin to be set")
		}
		return []byte("[main 1234567] feat: add login"), nil
	}}
	g := &Git{Exec: fakeExec}

	err := g.Commit("feat: add login\n\nAdds OAuth2 flow with PKCE.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommit_error(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("error: pathspec 'foo' did not match any file(s) known to git"), fmt.Errorf("exit status 1")
	}}
	g := &Git{Exec: fakeExec}

	err := g.Commit("feat: something")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "error: pathspec") {
		t.Errorf("error = %v, want error containing stderr text", err)
	}
}

func TestCommitAll(t *testing.T) {
	fakeExec := &fakeExecutor{fn: func(cmd *exec.Cmd) ([]byte, error) {
		if !slices.Contains(cmd.Args, "-a") {
			return nil, fmt.Errorf("expected -a flag in args, got %v", cmd.Args)
		}
		return []byte("[main 1234567] feat: changes"), nil
	}}
	g := &Git{Exec: fakeExec}

	err := g.CommitAll("feat: changes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
