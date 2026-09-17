package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/spf13/cobra"
)

var (
	troubleshootDbPath   string
	troubleshootClearAll bool
)

var troubleshootCmd = &cobra.Command{
	Use:   "troubleshoot",
	Short: "Troubleshooting utilities for reducarr",
	Long:  "Inspect database tables and perform maintenance operations such as clearing tables.",
	Run: func(cmd *cobra.Command, args []string) {
		runTroubleshootSummary(cmd.OutOrStdout(), troubleshootDbPath)
	},
}

var troubleshootClearTableCmd = &cobra.Command{
	Use:   "clear-table [table_name]",
	Short: "Clear records from a database table",
	Long:  "Clears all records from a specified database table or all tables with --all flag.",
	Run: func(cmd *cobra.Command, args []string) {
		out := cmd.OutOrStdout()
		errOut := cmd.ErrOrStderr()

		if troubleshootClearAll {
			runClearAllTables(out, errOut, troubleshootDbPath)
			return
		}

		if len(args) == 0 {
			fmt.Fprintln(errOut, "Error: specify a table name to clear or use --all flag.")
			_ = cmd.Help()
			return
		}

		tableName := args[0]
		runClearSingleTable(out, errOut, troubleshootDbPath, tableName)
	},
}

func runTroubleshootSummary(out io.Writer, dbPath string) {
	if dbPath == "" {
		dbPath = "reducarr.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(out, "Error opening database: %v\n", err)
		return
	}
	defer db.Close(database)

	counts, err := database.GetTableCounts()
	if err != nil {
		fmt.Fprintf(out, "Error fetching table counts: %v\n", err)
		return
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TABLE\tCOUNT")
	for _, table := range db.AllowedTables {
		fmt.Fprintf(w, "%s\t%d\n", table, counts[table])
	}
	_ = w.Flush()
}

func runClearSingleTable(out, errOut io.Writer, dbPath, tableName string) {
	if dbPath == "" {
		dbPath = "reducarr.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(errOut, "Error opening database: %v\n", err)
		return
	}
	defer db.Close(database)

	rows, err := database.ClearTable(tableName)
	if err != nil {
		fmt.Fprintf(errOut, "Error clearing table %q: %v\n", tableName, err)
		return
	}

	fmt.Fprintf(out, "Table %q cleared successfully. Deleted %d rows.\n", tableName, rows)
}

func runClearAllTables(out, errOut io.Writer, dbPath string) {
	if dbPath == "" {
		dbPath = "reducarr.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(errOut, "Error opening database: %v\n", err)
		return
	}
	defer db.Close(database)

	for _, table := range db.AllowedTables {
		rows, err := database.ClearTable(table)
		if err != nil {
			fmt.Fprintf(errOut, "Error clearing table %q: %v\n", table, err)
		} else {
			fmt.Fprintf(out, "Table %q cleared. Deleted %d rows.\n", table, rows)
		}
	}
}

var troubleshootWALCheckpointCmd = &cobra.Command{
	Use:   "wal-checkpoint",
	Short: "Flush and truncate the SQLite WAL file",
	Long:  "Runs PRAGMA wal_checkpoint(TRUNCATE) to commit all WAL frames into the main database file and truncate the WAL.",
	Run: func(cmd *cobra.Command, args []string) {
		runWALCheckpoint(cmd.OutOrStdout(), cmd.ErrOrStderr(), troubleshootDbPath)
	},
}

func runWALCheckpoint(out, errOut io.Writer, dbPath string) {
	if dbPath == "" {
		dbPath = "reducarr.db"
	}
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(errOut, "Error opening database: %v\n", err)
		return
	}
	defer db.Close(database)

	walFrames, checkpointed, err := database.CheckpointWAL()
	if err != nil {
		fmt.Fprintf(errOut, "Error running WAL checkpoint: %v\n", err)
		return
	}
	fmt.Fprintf(out, "WAL checkpoint complete: %d/%d frames checkpointed.\n", checkpointed, walFrames)
}

func init() {
	troubleshootCmd.PersistentFlags().StringVar(&troubleshootDbPath, "db", "reducarr.db", "Path to database file")
	troubleshootClearTableCmd.Flags().BoolVar(&troubleshootClearAll, "all", false, "Clear all database tables")

	troubleshootCmd.AddCommand(troubleshootClearTableCmd)
	troubleshootCmd.AddCommand(troubleshootWALCheckpointCmd)
	rootCmd.AddCommand(troubleshootCmd)
}
