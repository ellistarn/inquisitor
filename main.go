package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	inquisitor "github.com/ellistarn/inquisitor/pkg"
)

//go:embed SKILL.md
var skillFile []byte

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install" {
		if err := installSkill(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

func installSkill() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".agents", "skills", "inquisition")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, skillFile, 0644); err != nil {
		return err
	}
	fmt.Printf("installed %s\n", path)
	return nil
}
