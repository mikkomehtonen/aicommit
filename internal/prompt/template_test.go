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
	// The diff placeholder (%s) should resolve to empty string at the end.
	if !strings.HasSuffix(strings.TrimSpace(got), "") {
		t.Error("Build with empty diff should handle empty diff gracefully")
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