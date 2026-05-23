package git

import (
	"fmt"
	"os/exec"
)

// Commit creates a git commit with the given message.
func Commit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running git commit: %w\n%s", err, string(out))
	}
	return nil
}

// CommitAll creates a git commit with the -a flag (stages all tracked files) and the given message.
func CommitAll(message string) error {
	cmd := exec.Command("git", "commit", "-am", message)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("running git commit -a: %w\n%s", err, string(out))
	}
	return nil
}