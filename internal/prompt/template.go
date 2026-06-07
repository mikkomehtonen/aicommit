package prompt

import (
	"strings"
)

const template = `You are an expert software engineer.
Generate a concise Conventional Commit message from the following git diff.
Return only the commit message.
Do not use markdown.

{DIFF}`

const retryTemplate = `You are an expert software engineer.
Generate a concise Conventional Commit message from the following git diff.
Return only the commit message.
Do not use markdown.

The user rejected these previous suggestions:
{SUGGESTIONS}
Generate a different Conventional Commit message. Do not reuse the same wording.

{DIFF}`

const rewordTemplate = `You are an expert software engineer.
The current commit message is:
{CURRENT_MESSAGE}

Generate an improved, concise Conventional Commit message based on the following git diff.
Return only the commit message.
Do not use markdown.

{DIFF}`

// Build constructs the prompt by injecting the diff into the template.
func Build(diff string) string {
	return strings.Replace(template, "{DIFF}", diff, 1)
}

// BuildRetry constructs a retry prompt that includes previously rejected
// suggestions so the model generates a different commit message.
func BuildRetry(diff string, previousSuggestions []string) string {
	var sb strings.Builder
	for _, s := range previousSuggestions {
		sb.WriteString("- ")
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	res := strings.Replace(retryTemplate, "{SUGGESTIONS}", sb.String(), 1)
	return strings.Replace(res, "{DIFF}", diff, 1)
}

// BuildReword constructs a prompt for rewording an existing commit message,
// including the current message and the diff of the commit.
func BuildReword(diff, currentMessage string) string {
	res := strings.Replace(rewordTemplate, "{CURRENT_MESSAGE}", currentMessage, 1)
	return strings.Replace(res, "{DIFF}", diff, 1)
}

// BuildRewordRetry constructs a retry prompt for rewording, including previously
// rejected suggestions, the current commit message, and the diff.
func BuildRewordRetry(diff, currentMessage string, previousSuggestions []string) string {
	var sb strings.Builder
	for _, s := range previousSuggestions {
		sb.WriteString("- ")
		sb.WriteString(s)
		sb.WriteString("\n")
	}
	base := rewordTemplate + "\nThe user rejected these previous suggestions:\n" + sb.String() + "\nGenerate a different Conventional Commit message. Do not reuse the same wording."
	res := strings.Replace(base, "{CURRENT_MESSAGE}", currentMessage, 1)
	return strings.Replace(res, "{DIFF}", diff, 1)
}
