package main

import (
	"errors"
	"fmt"
	"os"

	"slurm_monitor/internal/app"
	"slurm_monitor/internal/config"
)

func main() {
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
