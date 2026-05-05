package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	inquisitor "github.com/ellistarn/inquisitor/pkg"
	"github.com/spf13/cobra"
)

//go:embed SKILL.md
var skillFile []byte

func main() {
	rootCmd := &cobra.Command{
		Use:   "inquisitor [patterns...]",
		Short: "Static analysis report for Go codebases",
		Long: `Produces a self-contained complexity report for AI agents that compare metrics
against design documents. Run against Go packages to identify where complexity lives
relative to where designs say it should be.

Analyzes all packages matching the given patterns (default: ./...).`,
		Args:             cobra.ArbitraryArgs,
		TraverseChildren: true,
		SilenceUsage:     true,
		SilenceErrors:    true,
		RunE: func(cmd *cobra.Command, args []string) error {
			patterns := args
			if len(patterns) == 0 {
				patterns = []string{"./..."}
			}

			pkgs, err := inquisitor.LoadPackages(patterns)
			if err != nil {
				return err
			}

			analyzedPaths := inquisitor.BuildAnalyzedPaths(pkgs)
			functions := inquisitor.AnalyzeFunctions(pkgs, analyzedPaths)
			types := inquisitor.AnalyzeTypes(pkgs, analyzedPaths)
			mod, packages := inquisitor.AnalyzePackages(pkgs, functions, types, analyzedPaths)
			inquisitor.GenerateReport(os.Stdout, mod, packages, functions, types)
			return nil
		},
	}

	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the inquisition agent skill",
		Long: `Writes the embedded SKILL.md to ~/.agents/skills/inquisition/SKILL.md for use
by AI coding agents.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return installSkill()
		},
	}

	rootCmd.AddCommand(installCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

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
