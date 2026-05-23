package git

import (
	"fmt"
	"os/exec"
	"strings"
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
		if strings.Contains(string(out), "ambiguous argument 'HEAD'") {
			return "", fmt.Errorf("no commits yet; use --staged or make an initial commit first")
		}
		return "", fmt.Errorf("running git diff HEAD: %w\n%s", err, string(out))
	}
	return string(out), nil
}