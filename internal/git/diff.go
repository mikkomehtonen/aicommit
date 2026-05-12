package git

import (
	"fmt"
	"os/exec"
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