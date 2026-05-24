package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of reducarr",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("reducarr v0.0.1-alpha")
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
