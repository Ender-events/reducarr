package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Inspect the internal database",
}

var dbStateCmd = &cobra.Command{
	Use:   "scan-state",
	Short: "Display the scanner state (checkpoints)",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		states, err := database.GetAllScanStates()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching scan states: %v\n", err)
			os.Exit(1)
		}

		if len(states) == 0 {
			fmt.Println("No scan states found.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "INSTANCE ID\tLAST ITEM ID\tUPDATED AT")
		for _, s := range states {
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.InstanceID, s.LastItemID, s.UpdatedAt)
		}
		w.Flush()
	},
}

var dbSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show a summary of the database content (record counts)",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		tables := []string{"scan_state", "torrents", "media_files", "candidates", "reports"}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TABLE\tCOUNT")

		for _, table := range tables {
			var count int
			err := database.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
			if err != nil {
				fmt.Fprintf(w, "%s\tERROR: %v\n", table, err)
			} else {
				fmt.Fprintf(w, "%s\t%d\n", table, count)
			}
		}
		w.Flush()
	},
}

func init() {
	dbCmd.AddCommand(dbStateCmd)
	dbCmd.AddCommand(dbSummaryCmd)
	rootCmd.AddCommand(dbCmd)
}
