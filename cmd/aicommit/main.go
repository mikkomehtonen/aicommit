package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"aicommit/internal/git"
	"aicommit/internal/llm"
	"aicommit/internal/prompt"

	"github.com/spf13/cobra"
)

var commitFlag bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "aicommit",
		Short: "Generate Conventional Commit messages using a local LLM",
		Long:  "aicommit reads your staged git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout.",
		RunE:  run,
	}

	rootCmd.Flags().BoolVarP(&commitFlag, "commit", "c", false, "prompt to accept/retry and commit the generated message")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	diff, err := git.StagedDiff()
	if err != nil {
		return fmt.Errorf("getting staged diff: %w", err)
	}

	if strings.TrimSpace(diff) == "" {
		fmt.Fprintln(os.Stderr, "No staged changes found. Stage your changes with: git add <files>")
		os.Exit(1)
	}

	if !commitFlag {
		msg, err := llm.Generate(prompt.Build(diff))
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		fmt.Println(strings.TrimSpace(msg))
		return nil
	}

	return interactiveCommit(diff)
}

func interactiveCommit(diff string) error {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		msg, err := llm.Generate(prompt.Build(diff))
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		msg = strings.TrimSpace(msg)

		fmt.Println(msg)
		fmt.Print("[a]ccept, [r]etry, [c]ancel: ")

		if !scanner.Scan() {
			// EOF or signal
			fmt.Println()
			return nil
		}

		choice := strings.TrimSpace(scanner.Text())
		switch choice {
		case "a", "":
			if msg == "" {
				fmt.Fprintln(os.Stderr, "Error: generated commit message is empty, retrying.")
				continue
			}
			if err := git.Commit(msg); err != nil {
				return fmt.Errorf("committing: %w", err)
			}
			return nil
		case "r":
			continue
		case "c":
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		default:
			fmt.Fprintf(os.Stderr, "Unknown choice %q, use [a]ccept, [r]etry, or [c]ancel.\n", choice)
		}
	}
}