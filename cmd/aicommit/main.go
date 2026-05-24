package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

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
	Generate(ctx context.Context, prompt string) (string, error)
}

// MessageGeneratorWithTemperature sends a prompt to the LLM with a given
// temperature and returns the response.
type MessageGeneratorWithTemperature interface {
	GenerateWithTemperature(ctx context.Context, prompt string, temperature float64) (string, error)
}

// RunConfig holds the dependencies and settings for run.
type RunConfig struct {
	DiffProvider     DiffProvider
	Generator        MessageGenerator
	Committer        Committer
	Stdin            io.Reader
	Stdout           io.Writer
	Stderr           io.Writer
	Temperature      float64
	RetryTemperature float64
	AllFlag          bool
	CommitFlag       bool
}

var version = "dev"

func main() {
	var (
		commitFlag           bool
		allFlag              bool
		temperatureFlag      float64
		retryTemperatureFlag float64
		timeoutFlag          string
	)

	rootCmd := &cobra.Command{
		Use:     "aicommit",
		Short:   "Generate Conventional Commit messages using a local LLM",
		Long:    "aicommit reads your staged git diff, sends it to an OpenAI-compatible LLM API, and prints a Conventional Commit message to stdout.\n\nUse --all to include all changes (staged + unstaged) instead of only staged changes.",
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, warnings := llm.NewClient()
			for _, w := range warnings {
				fmt.Fprintln(os.Stderr, w)
			}
if cmd.Flags().Changed("timeout") {
			d, err := time.ParseDuration(timeoutFlag)
			if err != nil {
				return fmt.Errorf("invalid --timeout duration %q: %w", timeoutFlag, err)
			}
			client.Timeout = d
		}
			temp := resolveTemperature(temperatureFlag, cmd.Flags().Changed("temperature"), client.Temperature)
			retryTemp := resolveTemperature(retryTemperatureFlag, cmd.Flags().Changed("retry-temperature"), client.RetryTemperature)
			g := git.New()
			err := run(cmd.Context(), RunConfig{
				DiffProvider:     g,
				Generator:        client,
				Committer:        g,
				Stdin:            os.Stdin,
				Stdout:           os.Stdout,
				Stderr:           os.Stderr,
				Temperature:      temp,
				RetryTemperature: retryTemp,
				AllFlag:          allFlag,
				CommitFlag:       commitFlag,
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
	rootCmd.Flags().StringVar(&timeoutFlag, "timeout", "", "HTTP timeout as a Go duration string, e.g. \"60s\" or \"2m\" (default: AICOMMIT_TIMEOUT env var, or 60s)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg RunConfig) error {
	diff, err := diffForMode(cfg.DiffProvider, cfg.AllFlag)
	if err != nil {
		return err
	}

	if strings.TrimSpace(diff) == "" {
		if cfg.AllFlag {
			fmt.Fprintln(cfg.Stderr, "No changes found.")
		} else {
			fmt.Fprintln(cfg.Stderr, "No staged changes found. Stage your changes with: git add <files>\nOr use --all to include all changes.")
		}
		return errEmptyDiff
	}

	if !cfg.CommitFlag {
		msg, err := generateWithFallback(ctx, cfg.Generator, prompt.Build(diff), cfg.Temperature)
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		fmt.Fprintln(cfg.Stdout, strings.TrimSpace(msg))
		return nil
	}

	return interactiveCommit(ctx, cfg, diff, cfg.AllFlag)
}

func diffForMode(dp DiffProvider, all bool) (string, error) {
	if all {
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

var errEmptyDiff = errors.New("empty diff")

// resolveTemperature returns the flag value if it was explicitly set, otherwise the default.
func resolveTemperature(flag float64, changed bool, defaultVal float64) float64 {
	if changed {
		return flag
	}
	return defaultVal
}

// generateWithFallback tries GenerateWithTemperature first, falling back to
// Generate if the generator does not support temperature.
func generateWithFallback(ctx context.Context, mg MessageGenerator, prompt string, temperature float64) (string, error) {
	if withTemp, ok := mg.(MessageGeneratorWithTemperature); ok {
		return withTemp.GenerateWithTemperature(ctx, prompt, temperature)
	}
	return mg.Generate(ctx, prompt)
}

func interactiveCommit(ctx context.Context, cfg RunConfig, diff string, all bool) error {
	scanner := bufio.NewScanner(cfg.Stdin)
	var previousSuggestions []string
	isRetry := false

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		msg, err := generateWithFallback(ctx, cfg.Generator, promptText, genTemp)
		if err != nil {
			return fmt.Errorf("generating commit message: %w", err)
		}
		msg = strings.TrimSpace(msg)

		// Confirmation loop: allows editing without regenerating.
	confirmLoop:
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
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
					previousSuggestions = append(previousSuggestions, msg)
					if len(previousSuggestions) > 5 {
						previousSuggestions = previousSuggestions[len(previousSuggestions)-5:]
					}
					isRetry = true
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
				if len(previousSuggestions) > 5 {
					previousSuggestions = previousSuggestions[len(previousSuggestions)-5:]
				}
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