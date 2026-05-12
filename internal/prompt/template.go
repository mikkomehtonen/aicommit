package prompt

import "fmt"

const template = `You are an expert software engineer.
Generate a concise Conventional Commit message from the following git diff.
Return only the commit message.
Do not use markdown.

%s`

// Build constructs the prompt by injecting the diff into the template.
func Build(diff string) string {
	return fmt.Sprintf(template, diff)
}