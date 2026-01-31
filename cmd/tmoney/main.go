package main

import (
	"fmt"
	"os"
)

// Version information - will be set via build flags in production
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	// Handle basic flags
	for _, arg := range args {
		switch arg {
		case "-v", "--version":
			printVersion()
			return nil
		case "-h", "--help":
			printHelp()
			return nil
		}
	}

	// Default to TUI mode (placeholder for now)
	fmt.Println("TMoney - Personal Finance Manager")
	fmt.Println("TUI mode not yet implemented.")
	fmt.Println("Use --help for available options.")

	return nil
}

func printVersion() {
	fmt.Printf("tmoney version %s\n", Version)
	fmt.Printf("Build time: %s\n", BuildTime)
	fmt.Printf("Git commit: %s\n", GitCommit)
}

func printHelp() {
	fmt.Println(`TMoney - Personal Finance Manager

Usage:
  tmoney [file.tdb]              Launch TUI with optional file
  tmoney [options]               Run CLI commands

Global Options:
  -f, --file <path>    Specify database file
  -h, --help           Show this help message
  -v, --version        Show version information

For more information, visit: https://github.com/haskovec/tmoney`)
}
