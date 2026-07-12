package main

import (
	"fmt"

	"github.com/Ender-events/reducarr/internal/buildinfo"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of reducarr",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("reducarr %s (commit: %s, built with %s, at %s)\n",
			buildinfo.Version,
			buildinfo.Commit,
			buildinfo.GoVersion(),
			buildinfo.BuildTime,
		)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
