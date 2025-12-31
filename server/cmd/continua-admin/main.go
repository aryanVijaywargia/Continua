package main

import (
	"fmt"
	"os"

	"github.com/aryanVijaywargia/Continua/server/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println(version.String())
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Continua Admin CLI")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  continua-admin <command>")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  version     Show version information")
	fmt.Println("  help        Show this help message")
}
