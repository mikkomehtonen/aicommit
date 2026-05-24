package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Executor runs an exec.Cmd and returns combined output.
type Executor interface {
	CombinedOutput(cmd *exec.Cmd) ([]byte, error)
}

type defaultExecutor struct{}

func (defaultExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}

// Git provides git operations with an injectable executor for testing.
type Git struct {
	Exec Executor
}

// New returns a Git with the default (real) executor.
func New() *Git {
	return &Git{Exec: defaultExecutor{}}
}

// StagedDiff returns the output of `git diff --staged`.
func (g *Git) StagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("running git diff --staged: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// AllDiff returns the output of `git diff HEAD`.
func (g *Git) AllDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		if hasExitCode(err, 128) {
			stderr := strings.TrimSpace(string(out))
			if strings.Contains(stderr, "bad revision 'HEAD'") ||
				strings.Contains(stderr, "does not have any commits") ||
				strings.Contains(stderr, "unknown revision or path not in the working tree") {
				return "", nil
			}
		}
		return "", fmt.Errorf("running git diff HEAD: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

type exitCoder interface {
	ExitCode() int
}

func hasExitCode(err error, code int) bool {
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode() == code
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}
