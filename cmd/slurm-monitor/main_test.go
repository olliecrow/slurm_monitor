package main

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

func TestCompletionHelpTextIncludesUsage(t *testing.T) {
	help := completionHelpText()
	for _, want := range []string{
		"slurm-monitor completion",
		"Usage:",
		"mkdir -p ~/.local/share/bash-completion/completions",
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

func TestCompletionScriptsDoNotAdvertiseUnsupportedHelpCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		script, err := completionScript(shell)
		if err != nil {
			t.Fatalf("completionScript(%q): %v", shell, err)
		}
		if strings.Contains(script, "help:") || strings.Contains(script, " monitor help") {
			t.Fatalf("%s completion advertises unsupported help command:\n%s", shell, script)
		}
	}
}

func TestBashCompletesFlagsAtAnyPosition(t *testing.T) {
	script, err := completionScript("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, position := range []int{1, 2, 3} {
		setup := fmt.Sprintf("\n_init_completion() { cur=--re; cword=%d; words=(slurm-monitor host --re); }\n_slurm_monitor_completion\nprintf '%%s\\n' \"${COMPREPLY[@]}\"\n", position)
		cmd := exec.Command("bash")
		cmd.Stdin = strings.NewReader(script + setup)
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "--refresh" {
			t.Fatalf("position %d: %q, %v", position, out, err)
		}
	}
}
