package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	noColor      bool
	outputFormat string
)

var rootCmd = &cobra.Command{
	Use:   "vlt",
	Short: "vlt CLI tool",
	Long:  `vlt is a command line tool for managing secrets and configuration.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("vlt %s\n", rootCmd.Version)
		fmt.Printf("  commit: %s\n", versionCommit)
		fmt.Printf("  built:  %s\n", versionDate)
	},
}

var (
	versionCommit string
	versionDate   string
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "output", "text", "output format: text, json, yaml")
	rootCmd.AddCommand(versionCmd)
}

func Execute(version, commit, date string) {
	rootCmd.Version = version
	versionCommit = commit
	versionDate = date

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
