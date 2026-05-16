package prompt

import (
	"fmt"
	"strings"
)

const template = `You are an expert software engineer.
Generate a concise Conventional Commit message from the following git diff.
Return only the commit message.
Do not use markdown.

%s`

const retryTemplate = `You are an expert software engineer.
Generate a concise Conventional Commit message from the following git diff.
Return only the commit message.
Do not use markdown.

The user rejected these previous suggestions:
%s
Generate a different Conventional Commit message. Do not reuse the same wording.

%s`

// Build constructs the prompt by injecting the diff into the template.
func Build(diff string) string {
	return fmt.Sprintf(template, diff)
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
	return fmt.Sprintf(retryTemplate, sb.String(), diff)
}