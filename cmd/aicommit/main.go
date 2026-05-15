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

// DiffProvider retrieves a git diff.
type DiffProvider interface {
	StagedDiff() (string, error)
	AllDiff() (string, error)
}

// Committer creates a git commit.
type Committer interface {
	Commit(message string) error
	CommitAll(message string) error
}

// MessageGenerator sends a prompt to the LLM and returns the response.
type MessageGenerator interface {
	Generate(prompt string) (string, error)
}

// MessageGeneratorWithTemperature sends a prompt to the LLM with a given
// temperature and returns the response.
type MessageGeneratorWithTemperature interface {
	GenerateWithTemperature(prompt string, temperature float64) (string, error)
}

// realGit wraps the internal/git package to satisfy DiffProvider and Committer.
type realGit struct{}

func (realGit) StagedDiff() (string, error) { return git.StagedDiff() }
func (realGit) AllDiff() (string, error)    { return git.AllDiff() }
func (realGit) Commit(msg string) error      { return git.Commit(msg) }
func (realGit) CommitAll(msg string) error    { return git.CommitAll(msg) }

var commitFlag bool
var allFlag bool
var temperatureFlag float64
var retryTemperatureFlag float64

func main() {
	rootCmd := &cobra.Command{
		Use:   "aicommit",
		Short: "Generate Conventional Commit messages using a local LLM",
		Long:  "aicommit reads your staged git diff, sends it to a local LM Studio instance, and prints a Conventional Commit message to stdout.\n\nUse --all to include all changes (staged + unstaged) instead of only staged changes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(realGit{}, llm.NewClient(), os.Stdin, os.Stdout, os.Stderr, temperatureFlag, retryTemperatureFlag)
		},
	}

	rootCmd.Flags().BoolVarP(&commitFlag, "commit", "c", false, "prompt to accept/retry and commit the generated message")
	rootCmd.Flags().BoolVarP(&allFlag, "all", "a", false, "include all changes (staged + unstaged) via git diff HEAD")
	rootCmd.Flags().Float64Var(&temperatureFlag, "temperature", 0, "temperature for the first request (default: AICOMMIT_TEMPERATURE env var, or 0.1)")
	rootCmd.Flags().Float64Var(&retryTemperatureFlag, "retry-temperature", 0, "temperature for retry requests (default: AICOMMIT_RETRY_TEMPERATURE env var, or 0.4)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(dp DiffProvider, mg MessageGenerator, stdin io.Reader, stdout, stderr io.Writer, temperature, retryTemperature float64) error {
	diff, err := diffForMode(dp)
	if err != nil {
		return err
	}

	if strings.TrimSpace(diff) == "" {
		if allFlag {
			fmt.Fprintln(stderr, "No changes found.")
		} else {
			fmt.Fprintln(stderr, "No staged changes found. Stage your changes with: git add <files>\nOr use --all to include all changes.")
		}
		return errEmptyDiff
	}

	if !commitFlag {
		msg, err := generateWithFallback(mg, prompt.Build(diff), temperature)
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		fmt.Fprintln(stdout, strings.TrimSpace(msg))
		return nil
	}

	return interactiveCommit(diff, mg, realGit{}, allFlag, stdin, stdout, stderr, temperature, retryTemperature)
}

func diffForMode(dp DiffProvider) (string, error) {
	if allFlag {
		diff, err := dp.AllDiff()
		if err != nil {
			return "", fmt.Errorf("getting diff: %w", err)
		}
		return diff, nil
	}
	diff, err := dp.StagedDiff()
	if err != nil {
		return "", fmt.Errorf("getting staged diff: %w", err)
	}
	return diff, nil
}

var errEmptyDiff = fmt.Errorf("empty diff")

// generateWithFallback tries GenerateWithTemperature first, falling back to
// Generate if the generator does not support temperature.
func generateWithFallback(mg MessageGenerator, prompt string, temperature float64) (string, error) {
	if withTemp, ok := mg.(MessageGeneratorWithTemperature); ok {
		return withTemp.GenerateWithTemperature(prompt, temperature)
	}
	return mg.Generate(prompt)
}

func interactiveCommit(diff string, mg MessageGenerator, c Committer, all bool, stdin io.Reader, stdout, stderr io.Writer, temperature, retryTemperature float64) error {
	scanner := bufio.NewScanner(stdin)
	var previousSuggestions []string
	isRetry := false

	for {
		var promptText string
		if isRetry {
			promptText = prompt.BuildRetry(diff, previousSuggestions)
		} else {
			promptText = prompt.Build(diff)
		}
		genTemp := temperature
		if isRetry {
			genTemp = retryTemperature
		}
		msg, err := generateWithFallback(mg, promptText, genTemp)
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
				if all {
					if err := c.CommitAll(msg); err != nil {
						return fmt.Errorf("committing: %w", err)
					}
				} else {
					if err := c.Commit(msg); err != nil {
						return fmt.Errorf("committing: %w", err)
					}
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
				previousSuggestions = append(previousSuggestions, msg)
				isRetry = true
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