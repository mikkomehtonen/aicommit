package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// StagedDiff returns the output of `git diff --staged`.
func StagedDiff() (string, error) {
	cmd := exec.Command("git", "diff", "--staged")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("running git diff --staged: %w", err)
	}
	return string(out), nil
}

// AllDiff returns the output of `git diff HEAD`.
func AllDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(err.Error(), "ambiguous argument 'HEAD'") {
			return "", fmt.Errorf("no commits yet; use --staged or make an initial commit first")
		}
		return "", fmt.Errorf("running git diff HEAD: %w", err)
	}
	return string(out), nil
}