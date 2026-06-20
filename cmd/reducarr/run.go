package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/orchestrator"
	"github.com/spf13/cobra"
)

var autoUpgrade bool

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the background automation loop",
	Long:  `Run Reducarr in headless mode, executing scheduled scans and optimizations.`,
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		autoUpgradeVal := cfg.Automation.AutoUpgrade
		if cmd.Flags().Changed("auto-upgrade") {
			autoUpgradeVal = autoUpgrade
		}

		manager, err := orchestrator.NewAutomationManager(database, verbose, dryRun, autoUpgradeVal)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating automation manager: %v\n", err)
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\n\033[33mStopping automation...\033[0m")
			cancel()
		}()

		if err := manager.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Automation error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Do not perform any destructive actions (torrent deletion, release grab)")
	runCmd.Flags().BoolVar(&autoUpgrade, "auto-upgrade", false, "Override auto-upgrade setting from config")
	rootCmd.AddCommand(runCmd)
}
