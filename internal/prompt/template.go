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
