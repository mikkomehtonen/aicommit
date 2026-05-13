package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"aicommit/internal/git"
	"aicommit/internal/llm"
	"aicommit/internal/prompt"

	"github.com/spf13/cobra"
)

// DiffProvider retrieves the staged git diff.
type DiffProvider interface {
	StagedDiff() (string, error)
}

// Committer creates a git commit.
type Committer interface {
	Commit(message string) error
}

// MessageGenerator sends a prompt to the LLM and returns the response.
type MessageGenerator interface {
	Generate(prompt string) (string, error)
}

// realGit wraps the internal/git package to satisfy DiffProvider and Committer.
type realGit struct{}

func (realGit) StagedDiff() (string, error) { return git.StagedDiff() }
func (realGit) Commit(msg string) error      { return git.Commit(msg) }

var commitFlag bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "aicommit",
		Short: "Generate Conventional Commit messages using a local LLM",
		Long:  "aicommit reads your staged git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(realGit{}, llm.NewClient(), os.Stdin, os.Stdout, os.Stderr)
		},
	}

	rootCmd.Flags().BoolVarP(&commitFlag, "commit", "c", false, "prompt to accept/retry and commit the generated message")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(dp DiffProvider, mg MessageGenerator, stdin io.Reader, stdout, stderr io.Writer) error {
	diff, err := dp.StagedDiff()
	if err != nil {
		return fmt.Errorf("getting staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		fmt.Fprintln(stderr, "No staged changes found. Stage your changes with: git add <files>")
		return errEmptyDiff
	}

	if !commitFlag {
		msg, err := mg.Generate(prompt.Build(diff))
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		fmt.Fprintln(stdout, strings.TrimSpace(msg))
		return nil
	}

	return interactiveCommit(diff, mg, realGit{}, stdin, stdout, stderr)
}

var errEmptyDiff = fmt.Errorf("empty diff")

func interactiveCommit(diff string, mg MessageGenerator, c Committer, stdin io.Reader, stdout, stderr io.Writer) error {
	scanner := bufio.NewScanner(stdin)

	for {
		msg, err := mg.Generate(prompt.Build(diff))
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		msg = strings.TrimSpace(msg)

		// Confirmation loop: allows editing without regenerating.
	confirmLoop:
		for {
			fmt.Fprintln(stdout, msg)
			fmt.Fprint(stdout, "[a]ccept, [e]dit, [r]etry, [c]ancel: ")

			if !scanner.Scan() {
				fmt.Fprintln(stdout)
				return nil
			}

			choice := strings.TrimSpace(scanner.Text())
			switch choice {
			case "a", "":
				if msg == "" {
					fmt.Fprintln(stderr, "Error: generated commit message is empty, retrying.")
					break confirmLoop
				}
				if err := c.Commit(msg); err != nil {
					return fmt.Errorf("committing: %w", err)
				}
				return nil
			case "e":
				fmt.Fprint(stdout, "Edit message: ")
				if !scanner.Scan() {
					fmt.Fprintln(stdout)
					return nil
				}
				edited := strings.TrimSpace(scanner.Text())
				if edited != "" {
					msg = edited
				}
				continue
			case "r":
				break confirmLoop
			case "c":
				fmt.Fprintln(stderr, "Cancelled.")
				return nil
			default:
				fmt.Fprintf(stderr, "Unknown choice %q, use [a]ccept, [e]dit, [r]etry, or [c]ancel.\n", choice)
			}
		}
	}
}