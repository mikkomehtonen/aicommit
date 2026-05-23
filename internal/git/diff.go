package git

import (
	"errors"
	"fmt"
	"os/exec"
)

// StagedDiff returns the output of `git diff --staged`.
func StagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running git diff --staged: %w\n%s", err, string(out))
	}
	return string(out), nil
}

// AllDiff returns the output of `git diff HEAD`.
func AllDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
			return "", nil
		}
		return "", fmt.Errorf("running git diff HEAD: %w\n%s", err, string(out))
	}
	return string(out), nil
}