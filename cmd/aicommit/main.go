package main

import (
	"fmt"
	"os"
	"strings"

	"aicommit/internal/git"
	"aicommit/internal/llm"
	"aicommit/internal/prompt"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "aicommit",
		Short: "Generate Conventional Commit messages using a local LLM",
		Long:  "aicommit reads your staged git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout.",
		RunE:  run,
	}

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

	msg, err := llm.Generate(prompt.Build(diff))
	if err != nil {
		return fmt.Errorf("generating commit message: %w", err)
	}

	fmt.Println(strings.TrimSpace(msg))
	return nil
}