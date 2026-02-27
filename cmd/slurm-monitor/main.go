package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"slurm_monitor/internal/app"
	"slurm_monitor/internal/config"
)

func main() {
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) == "completion" {
		os.Exit(runCompletion(os.Args[2:]))
	}

	cfg, err := config.ParseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, config.ErrHelpRequested) {
			fmt.Fprint(os.Stdout, config.HelpText())
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		fmt.Fprintln(os.Stderr, "run 'slurm-monitor --help' for usage details")
		os.Exit(2)
	}

	if err := app.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "slurm-monitor error: %v\n", err)
		os.Exit(1)
	}
}

func runCompletion(args []string) int {
	if len(args) >= 1 && isHelpArg(args[0]) {
		fmt.Fprint(os.Stdout, completionHelpText())
		return 0
	}
	if len(args) > 1 {
		fmt.Fprintln(os.Stderr, "argument error: completion accepts zero or one shell argument (bash or zsh)")
		return 2
	}
	shell := "bash"
	if len(args) == 1 {
		shell = strings.ToLower(strings.TrimSpace(args[0]))
	}
	script, err := completionScript(shell)
	if err != nil {
		fmt.Fprintf(os.Stderr, "argument error: %v\n", err)
		return 2
	}
	fmt.Fprint(os.Stdout, script)
	return 0
}

func isHelpArg(arg string) bool {
	return arg == "-h" || arg == "--help" || arg == "help"
}

func completionHelpText() string {
	return `slurm-monitor completion

Print shell completion script output for slurm-monitor.

Usage:
  slurm-monitor completion [bash|zsh]

Examples:
  slurm-monitor completion bash > ~/.local/share/bash-completion/completions/slurm-monitor
  mkdir -p ~/.zsh/completions
  slurm-monitor completion zsh > ~/.zsh/completions/_slurm-monitor
`
}

func completionScript(shell string) (string, error) {
	switch shell {
	case "bash":
		return `# bash completion for slurm-monitor
_slurm_monitor_completion() {
  local cur prev words cword
  _init_completion || return
  local commands="doctor dry-run completion monitor help"
  if [[ ${cword} -eq 1 ]]; then
    COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
    return
  fi
  case "${words[1]}" in
    completion)
      COMPREPLY=( $(compgen -W "bash zsh" -- "${cur}") )
      ;;
    doctor|dry-run|monitor)
      COMPREPLY=( $(compgen -W "--refresh --connect-timeout --command-timeout --ssh-config --identity-file --port --no-color --compact --once --duration" -- "${cur}") )
      ;;
    *)
      COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
      ;;
  esac
}
complete -F _slurm_monitor_completion slurm-monitor
`, nil
	case "zsh":
		return `#compdef slurm-monitor
_slurm_monitor() {
  local -a commands
  commands=(
    'monitor:start live monitoring (default)'
    'doctor:run non-mutating preflight checks'
    'dry-run:print planned execution order'
    'completion:print shell completion script'
    'help:show help text'
  )
  if (( CURRENT == 2 )); then
    _describe 'command' commands
    return
  fi
  case "${words[2]}" in
    completion)
      _values 'shell' bash zsh
      ;;
    doctor|dry-run|monitor)
      _values 'flag' --refresh --connect-timeout --command-timeout --ssh-config --identity-file --port --no-color --compact --once --duration
      ;;
    *)
      _message 'optional ssh target'
      ;;
  esac
}
_slurm_monitor "$@"
`, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (expected bash or zsh)", shell)
	}
}
