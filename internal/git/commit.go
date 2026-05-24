package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Commit creates a git commit with the given message.
func (g *Git) Commit(message string) error {
	cmd := exec.Command("git", "commit", "-F", "-")
	cmd.Stdin = strings.NewReader(message)
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("running git commit: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CommitAll creates a git commit with the -a flag (stages all tracked files) and the given message.
func (g *Git) CommitAll(message string) error {
	cmd := exec.Command("git", "commit", "-a", "-F", "-")
	cmd.Stdin = strings.NewReader(message)
	out, err := g.Exec.CombinedOutput(cmd)
	if err != nil {
		return fmt.Errorf("running git commit -a: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
