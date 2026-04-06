package main

import (
	"fmt"
	"os"

	memex "github.com/shivamvarshney/memex/internal"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: memex <serve|mcp|hook <session-start|session-stop>>")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		memex.RunServe()
	case "mcp":
		memex.RunMCP()
	case "hook":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: memex hook <session-start|session-stop>")
			os.Exit(1)
		}
		memex.RunHook(os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
