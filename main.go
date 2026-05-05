package main

import (
	"fmt"
	"os"

	inquisitor "github.com/ellistarn/inquisitor/pkg"
)

func main() {
	patterns := os.Args[1:]
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	pkgs, err := inquisitor.LoadPackages(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	analyzedPaths := inquisitor.BuildAnalyzedPaths(pkgs)
	functions := inquisitor.AnalyzeFunctions(pkgs, analyzedPaths)
	types := inquisitor.AnalyzeTypes(pkgs, analyzedPaths)
	mod, packages := inquisitor.AnalyzePackages(pkgs, functions, types, analyzedPaths)
	cycles := inquisitor.DetectCycles(packages)
	inquisitor.GenerateReport(os.Stdout, mod, packages, functions, types, cycles)
}
