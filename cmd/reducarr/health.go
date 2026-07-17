package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check connectivity to Arrs and Torrent Client",
	Run: func(cmd *cobra.Command, args []string) {
		client, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting client: %v\n", err)
			os.Exit(1)
		}

		results := client.HealthCheck(context.Background())

		hasError := false
		for _, res := range results {
			status := "HEALTHY"
			if !res.Healthy {
				status = "FAILED"
				hasError = true
			}
			fmt.Printf("[%s] %-15s: %s", status, res.Type, res.Name)
			if res.Error != nil {
				fmt.Printf(" (Error: %v)", res.Error)
			}
			fmt.Println()
		}

		if hasError {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
