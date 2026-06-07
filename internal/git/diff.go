package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Executor runs an exec.Cmd and returns output.
type Executor interface {
	Output(cmd *exec.Cmd) ([]byte, error)
	CombinedOutput(cmd *exec.Cmd) ([]byte, error)
}

type defaultExecutor struct{}

func (defaultExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	return cmd.Output()
}

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

// unstagedDiff returns the output of `git diff` (unstaged changes).
func (g *Git) unstagedDiff() (string, error) {
	cmd := exec.Command("git", "diff")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("running git diff: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// AllDiff returns the output of `git diff HEAD`.
// If there is no HEAD (first commit), it falls back to combining
// StagedDiff and UnstagedDiff.
func (g *Git) AllDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		if hasExitCode(err, 128) {
			stderr := strings.TrimSpace(string(out))
			if strings.Contains(stderr, "bad revision 'HEAD'") ||
				strings.Contains(stderr, "does not have any commits") ||
				strings.Contains(stderr, "unknown revision or path not in the working tree") {
				staged, err := g.StagedDiff()
				if err != nil {
					return "", fmt.Errorf("getting staged diff (fallback): %w", err)
				}
				unstaged, err := g.unstagedDiff()
				if err != nil {
					return "", fmt.Errorf("getting unstaged diff (fallback): %w", err)
				}
				if staged != "" && unstaged != "" {
					return staged + "\n" + unstaged, nil
				}
				return staged + unstaged, nil
			}
		}
		return "", fmt.Errorf("running git diff HEAD: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

type exitCoder interface {
	ExitCode() int
}

// HeadDiff returns the diff of the most recent commit via `git show --format="" HEAD`.
// Returns an error if there are no commits yet.
func (g *Git) HeadDiff() (string, error) {
	cmd := exec.Command("git", "show", "--format=", "HEAD")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		if hasExitCode(err, 128) {
			stderr := strings.TrimSpace(string(out))
			if strings.Contains(stderr, "bad revision 'HEAD'") ||
				strings.Contains(stderr, "does not have any commits") ||
				strings.Contains(stderr, "unknown revision or path not in the working tree") {
				return "", fmt.Errorf("no commits exist in this repository")
			}
		}
		return "", fmt.Errorf("running git show HEAD: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// HeadMessage returns the commit message of the most recent commit via `git log -1 --format=%B`.
// The returned message has a trailing newline stripped. Returns an error if no commits exist.
func (g *Git) HeadMessage() (string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%B")
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		if hasExitCode(err, 128) {
			stderr := strings.TrimSpace(string(out))
			if strings.Contains(stderr, "bad revision 'HEAD'") ||
				strings.Contains(stderr, "does not have any commits") ||
				strings.Contains(stderr, "unknown revision or path not in the working tree") {
				return "", fmt.Errorf("no commits exist in this repository")
			}
		}
		return "", fmt.Errorf("running git log: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
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
