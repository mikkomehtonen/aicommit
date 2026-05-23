package git

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestStagedDiff(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "diff", "--staged"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		return []byte("staged diff output"), nil
	}

	out, err := StagedDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "staged diff output" {
		t.Errorf("got %q, want %q", out, "staged diff output")
	}
}

func TestAllDiff(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		wantArgs := []string{"git", "diff", "HEAD"}
		if !reflect.DeepEqual(cmd.Args, wantArgs) {
			return nil, fmt.Errorf("unexpected args: got %v, want %v", cmd.Args, wantArgs)
		}
		return []byte("all diff output"), nil
	}

	out, err := AllDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "all diff output" {
		t.Errorf("got %q, want %q", out, "all diff output")
	}
}

func TestAllDiff_noHEAD(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		return nil, exitError(128)
	}

	out, err := AllDiff()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("got %q, want empty string", out)
	}
}

func TestStagedDiff_error(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("fatal: not a git repository"), fmt.Errorf("exit status 128")
	}

	_, err := StagedDiff()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "fatal: not a git repository") {
		t.Errorf("error = %v, want error containing stderr text", err)
	}
}

func TestCommit(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
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
	}

	err := Commit("feat: add login\n\nAdds OAuth2 flow with PKCE.")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommit_error(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		return []byte("error: pathspec 'foo' did not match any file(s) known to git"), fmt.Errorf("exit status 1")
	}

	err := Commit("feat: something")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "error: pathspec") {
		t.Errorf("error = %v, want error containing stderr text", err)
	}
}

func TestCommitAll(t *testing.T) {
	oldExecRun := execRun
	defer func() { execRun = oldExecRun }()

	execRun = func(cmd *exec.Cmd) ([]byte, error) {
		if !contains(cmd.Args, "-a") {
			return nil, fmt.Errorf("expected -a flag in args, got %v", cmd.Args)
		}
		return []byte("[main 1234567] feat: changes"), nil
	}

	err := CommitAll("feat: changes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// exitError returns an *exec.ExitError with the given exit code by running
// a real shell command that exits with that code.
func exitError(code int) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return fmt.Errorf("failed to create exit error %d: %w", code, err)
}
