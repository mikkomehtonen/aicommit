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

// RunConfig holds the dependencies and settings for run.
type RunConfig struct {
	DiffProvider       DiffProvider
	Generator          MessageGenerator
	Committer          Committer
	Stdin              io.Reader
	Stdout             io.Writer
	Stderr             io.Writer
	Temperature        float64
	RetryTemperature   float64
}

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
			client := llm.NewClient()
			temp := temperatureFlag
			retryTemp := retryTemperatureFlag
			if !cmd.Flags().Changed("temperature") {
				temp = client.Temperature
			}
			if !cmd.Flags().Changed("retry-temperature") {
				retryTemp = client.RetryTemperature
			}
			err := run(RunConfig{
				DiffProvider:     realGit{},
				Generator:        client,
				Committer:        realGit{},
				Stdin:            os.Stdin,
				Stdout:           os.Stdout,
				Stderr:           os.Stderr,
				Temperature:      temp,
				RetryTemperature: retryTemp,
			})
			if err == errEmptyDiff {
				os.Exit(1)
			}
			return err
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

func run(cfg RunConfig) error {
	diff, err := diffForMode(cfg.DiffProvider)
	if err != nil {
		return err
	}

	if strings.TrimSpace(diff) == "" {
		if allFlag {
			fmt.Fprintln(cfg.Stderr, "No changes found.")
		} else {
			fmt.Fprintln(cfg.Stderr, "No staged changes found. Stage your changes with: git add <files>\nOr use --all to include all changes.")
		}
		return errEmptyDiff
	}

	if !commitFlag {
		msg, err := generateWithFallback(cfg.Generator, prompt.Build(diff), cfg.Temperature)
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		fmt.Fprintln(cfg.Stdout, strings.TrimSpace(msg))
		return nil
	}

	return interactiveCommit(cfg, diff, allFlag)
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

func interactiveCommit(cfg RunConfig, diff string, all bool) error {
	scanner := bufio.NewScanner(cfg.Stdin)
	var previousSuggestions []string
	isRetry := false

	for {
		var promptText string
		if isRetry {
			promptText = prompt.BuildRetry(diff, previousSuggestions)
		} else {
			promptText = prompt.Build(diff)
		}
		genTemp := cfg.Temperature
		if isRetry {
			genTemp = cfg.RetryTemperature
		}
		msg, err := generateWithFallback(cfg.Generator, promptText, genTemp)
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		msg = strings.TrimSpace(msg)

		// Confirmation loop: allows editing without regenerating.
	confirmLoop:
		for {
			fmt.Fprintln(cfg.Stdout, msg)
			fmt.Fprint(cfg.Stdout, "[a]ccept, [e]dit, [r]etry, [c]ancel: ")

			if !scanner.Scan() {
				fmt.Fprintln(cfg.Stdout)
				return nil
			}

			choice := strings.TrimSpace(scanner.Text())
			switch choice {
			case "a", "":
				if msg == "" {
					fmt.Fprintln(cfg.Stderr, "Error: generated commit message is empty, retrying.")
					break confirmLoop
				}
				if all {
					if err := cfg.Committer.CommitAll(msg); err != nil {
						return fmt.Errorf("committing: %w", err)
					}
				} else {
					if err := cfg.Committer.Commit(msg); err != nil {
						return fmt.Errorf("committing: %w", err)
					}
				}
				return nil
			case "e":
				fmt.Fprint(cfg.Stdout, "Edit message: ")
				if !scanner.Scan() {
					fmt.Fprintln(cfg.Stdout)
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
				fmt.Fprintln(cfg.Stderr, "Cancelled.")
				return nil
			default:
				fmt.Fprintf(cfg.Stderr, "Unknown choice %q, use [a]ccept, [e]dit, [r]etry, or [c]ancel.\n", choice)
			}
		}
	}
}