package prompt

import (
	"strings"
	"testing"
)

func TestBuild(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n+hello"

	got := Build(diff)

	if !strings.Contains(got, diff) {
		t.Errorf("Build(%q) does not contain the diff", diff)
	}
	if !strings.Contains(got, "Conventional Commit") {
		t.Errorf("Build(%q) does not contain prompt instructions", diff)
	}
	if !strings.Contains(got, "Do not use markdown") {
		t.Errorf("Build(%q) missing markdown instruction", diff)
	}
}

func TestBuild_emptyDiff(t *testing.T) {
	got := Build("")

	// Should still contain the template instructions.
	if !strings.Contains(got, "Conventional Commit") {
		t.Error("Build with empty diff should still contain prompt instructions")
	}
	// Template should render with the diff placeholder resolved to empty string.
	expectedSuffix := "\n"
	if !strings.HasSuffix(got, expectedSuffix) {
		t.Errorf("Build with empty diff should end with newline, got: %q", got)
	}
}

func TestBuild_largeDiff(t *testing.T) {
	// Ensure no formatting issues with diffs containing percent signs.
	diff := "added 100% coverage"

	got := Build(diff)

	if !strings.Contains(got, "100% coverage") {
		t.Errorf("Build should preserve percent signs, got: %s", got)
	}
}

func TestBuildRetry_singleSuggestion(t *testing.T) {
	diff := "diff --git a/main.go b/main.go\n+hello"
	previous := []string{"feat: first attempt"}

	got := BuildRetry(diff, previous)

	if !strings.Contains(got, diff) {
		t.Errorf("BuildRetry does not contain the diff")
	}
	if !strings.Contains(got, "feat: first attempt") {
		t.Errorf("BuildRetry does not contain the previous suggestion")
	}
	if !strings.Contains(got, "rejected") {
		t.Errorf("BuildRetry should mention 'rejected'")
	}
	if !strings.Contains(got, "different") {
		t.Errorf("BuildRetry should instruct to generate a different message")
	}
	if !strings.Contains(got, "Do not reuse the same wording") {
		t.Errorf("BuildRetry should instruct not to reuse wording")
	}
}

func TestBuildRetry_multipleSuggestions(t *testing.T) {
	diff := "some diff"
	previous := []string{"feat: add login", "fix: resolve timeout"}

	got := BuildRetry(diff, previous)

	if !strings.Contains(got, "- feat: add login") {
		t.Errorf("BuildRetry should list first suggestion with bullet, got: %s", got)
	}
	if !strings.Contains(got, "- fix: resolve timeout") {
		t.Errorf("BuildRetry should list second suggestion with bullet, got: %s", got)
	}
}

func TestBuildRetry_emptySuggestions(t *testing.T) {
	diff := "some diff"
	previous := []string{}

	got := BuildRetry(diff, previous)

	// Should still contain the diff and instructions.
	if !strings.Contains(got, diff) {
		t.Errorf("BuildRetry does not contain the diff")
	}
	if !strings.Contains(got, "Conventional Commit") {
		t.Errorf("BuildRetry should contain prompt instructions")
	}
}

func TestBuildRetry_preservesPercentSigns(t *testing.T) {
	diff := "added 100% coverage"
	previous := []string{"feat: 100% done"}

	got := BuildRetry(diff, previous)

	if !strings.Contains(got, "100% coverage") {
		t.Errorf("BuildRetry should preserve percent signs in diff, got: %s", got)
	}
	if !strings.Contains(got, "100% done") {
		t.Errorf("BuildRetry should preserve percent signs in suggestions, got: %s", got)
	}
}
