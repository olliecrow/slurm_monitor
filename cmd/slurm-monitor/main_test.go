package main

import (
	"strings"
	"testing"
)

func TestCompletionHelpTextIncludesUsage(t *testing.T) {
	help := completionHelpText()
	for _, want := range []string{
		"slurm-monitor completion",
		"Usage:",
		"slurm-monitor completion [bash|zsh]",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("completion help missing %q", want)
		}
	}
}

func TestCompletionScriptSupportsBashAndZsh(t *testing.T) {
	bashScript, err := completionScript("bash")
	if err != nil {
		t.Fatalf("unexpected bash error: %v", err)
	}
	if !strings.Contains(bashScript, "complete -F _slurm_monitor_completion slurm-monitor") {
		t.Fatalf("expected bash completion directive, got:\n%s", bashScript)
	}

	zshScript, err := completionScript("zsh")
	if err != nil {
		t.Fatalf("unexpected zsh error: %v", err)
	}
	if !strings.Contains(zshScript, "#compdef slurm-monitor") {
		t.Fatalf("expected zsh completion header, got:\n%s", zshScript)
	}
}

func TestCompletionScriptRejectsUnsupportedShell(t *testing.T) {
	_, err := completionScript("fish")
	if err == nil {
		t.Fatalf("expected unsupported shell error")
	}
	if !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected unsupported shell error, got %v", err)
	}
}
