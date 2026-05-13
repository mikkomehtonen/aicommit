package git

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Commit creates a git commit with the given message.
func Commit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running git commit: %w\n%s", err, stderr.String())
	}
	return nil
}

// CommitAll creates a git commit with the -a flag (stages all tracked files) and the given message.
func CommitAll(message string) error {
	cmd := exec.Command("git", "commit", "-am", message)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running git commit -a: %w\n%s", err, stderr.String())
	}
	return nil
}