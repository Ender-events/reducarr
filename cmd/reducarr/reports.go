package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var (
	limit  int
	offset int
	clear  bool
)

type displayReport struct {
	ID         int
	Date       string
	Action     string
	Title      string
	Status     string
	Saved      string
	NewRelease string
	IsExit     bool
	Record     db.ReportRecord
}


var reportsCmd = &cobra.Command{
	Use:   "reports",
	Short: "Browse and manage operation reports",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		if clear {
			confirm := promptui.Prompt{
				Label:     "Are you sure you want to clear ALL reports?",
				IsConfirm: true,
			}
			if _, err := confirm.Run(); err == nil {
				if err := database.ClearReports(); err != nil {
					fmt.Printf("\033[31m✘\033[0m Error clearing reports: %v\n", err)
				} else {
					fmt.Println("\033[32m✔\033[0m All reports cleared.")
				}
			}
			return
		}

		for {
			reports, err := database.GetReports(limit, offset)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching reports: %v\n", err)
				os.Exit(1)
			}

			if len(reports) == 0 && offset == 0 {
				fmt.Println("No reports found.")
				return
			}

			items := make([]displayReport, len(reports)+1)
			for i, r := range reports {
				saved := "n/a"
				if r.ActionType == "UPGRADE" && r.TotalSizeAfter > 0 {
					saved = humanize.Bytes(uint64(r.TotalSizeBefore - r.TotalSizeAfter))
				} else if r.ActionType == "DELETE" {
					saved = humanize.Bytes(uint64(r.TotalSizeBefore))
				}

				statusColor := "\033[32m" // Green
				if r.Status == "FAILED" {
					statusColor = "\033[31m" // Red
				}

				items[i] = displayReport{
					ID:         r.ID,
					Date:       r.CreatedAt,
					Action:     r.ActionType,
					Title:      r.ItemTitle,
					Status:     fmt.Sprintf("%s%s\033[0m", statusColor, r.Status),
					Saved:      saved,
					NewRelease: r.NewReleaseTitle,
					Record:     r,
				}
			}
			items[len(reports)] = displayReport{
				Title:  "\033[33mExit\033[0m",
				IsExit: true,
			}

			templates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\033[31m▶\033[0m {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Date | faint }} | {{ .Action | yellow }} | {{ .Title | cyan }} ({{ .Saved | green }}){{ end }}",
				Inactive: "  {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Date | faint }} | {{ .Action | yellow }} | {{ .Title | cyan }} ({{ .Saved | green }}){{ end }}",
				Selected: "\033[32m✔\033[0m {{ if .IsExit }}Exited{{ else }}Report: {{ .Title | cyan }} ({{ .Date | faint }}){{ end }}",
				Details: `
--------- Report Details ---------
{{ if .IsExit }}
{{ "Close the reports browser" | faint }}
{{ else }}
{{ "ID:" | faint }}	{{ .ID }}
{{ "Date:" | faint }}	{{ .Date }}
{{ "Action:" | faint }}	{{ .Action }}
{{ "Status:" | faint }}	{{ .Status }}
{{ "Saved:" | faint }}	{{ .Saved }}
{{ if .NewRelease }}{{ "New Release:" | faint }}	{{ .NewRelease }}{{ end }}
{{ end }}`,
			}

			prompt := promptui.Select{
				Label:     "Select a report to view",
				Items:     items,
				Templates: templates,
				Size:      10,
			}

			index, _, err := prompt.Run()
			if err != nil {
				if err == promptui.ErrInterrupt {
					return
				}
				fmt.Fprintf(os.Stderr, "Prompt failed: %v\n", err)
				return
			}

			selected := items[index]
			if selected.IsExit {
				return
			}

			handleReportAction(selected, database)
		}
	},
}

func handleReportAction(selected displayReport, database *db.DB) {
	for {
		actionPrompt := promptui.Select{
			Label: fmt.Sprintf("Action for Report #%d", selected.ID),
			Items: []string{"View Details", "Delete Report Entry", "Back"},
		}

		_, action, err := actionPrompt.Run()
		if err != nil {
			return
		}

		switch action {
		case "View Details":
			showFullReport(selected.Record)
		case "Delete Report Entry":
			confirm := promptui.Prompt{
				Label:     fmt.Sprintf("Delete report entry #%d?", selected.ID),
				IsConfirm: true,
			}
			if _, err := confirm.Run(); err == nil {
				_ = database.DeleteReport(selected.ID)
				return
			}
		case "Back":
			return
		}
	}
}

func showFullReport(r db.ReportRecord) {
	fmt.Printf("\n\033[1mREPORT #%d - %s\033[0m\n", r.ID, r.CreatedAt)
	fmt.Printf("------------------------------------------\n")
	fmt.Printf("%-15s %s\n", "Action:", r.ActionType)
	fmt.Printf("%-15s %s (%s)\n", "Instance:", r.ArrInstance, r.ArrType)
	fmt.Printf("%-15s %s\n", "Item:", r.ItemTitle)
	fmt.Printf("%-15s %s\n", "Status:", r.Status)
	if r.ErrorMessage != "" {
		fmt.Printf("%-15s \033[31m%s\033[0m\n", "Error:", r.ErrorMessage)
	}

	fmt.Printf("\n\033[1mSpace Impact:\033[0m\n")
	fmt.Printf("%-15s %s\n", "Before:", humanize.Bytes(uint64(r.TotalSizeBefore)))
	if r.ActionType == "UPGRADE" {
		fmt.Printf("%-15s %s\n", "After:", humanize.Bytes(uint64(r.TotalSizeAfter)))
		fmt.Printf("%-15s \033[32m%s\033[0m\n", "Net Saved:", humanize.Bytes(uint64(r.TotalSizeBefore-r.TotalSizeAfter)))
		fmt.Printf("%-15s %s\n", "New Release:", r.NewReleaseTitle)
		fmt.Printf("%-15s %s\n", "Indexer:", r.NewIndexer)
	} else {
		fmt.Printf("%-15s \033[32m%s\033[0m\n", "Net Saved:", humanize.Bytes(uint64(r.TotalSizeBefore)))
	}

	if r.DeletedFiles != "" {
		fmt.Printf("\n\033[1mDeleted Files:\033[0m\n")
		var files []map[string]any
		_ = json.Unmarshal([]byte(r.DeletedFiles), &files)
		for _, f := range files {
			size, _ := f["size"].(float64)
			inode, _ := f["inode"].(float64)
			path, _ := f["path"].(string)
			fmt.Printf("  - [%d] %s (%s)\n", uint64(inode), path, humanize.Bytes(uint64(size)))
		}
	}

	if r.DeletedTorrents != "" {
		fmt.Printf("\n\033[1mDeleted Torrents:\033[0m\n")
		var torrents []map[string]string
		_ = json.Unmarshal([]byte(r.DeletedTorrents), &torrents)
		for _, t := range torrents {
			fmt.Printf("  - %s (Hash: %s)\n", t["client_name"], t["info_hash"][:8])
		}
	}
	fmt.Printf("------------------------------------------\n")
	fmt.Println("\033[2mPress enter to return...\033[0m")
	var dummy string
	fmt.Scanln(&dummy)
}

func init() {
	reportsCmd.Flags().IntVar(&limit, "limit", 50, "Limit number of reports to show")
	reportsCmd.Flags().IntVar(&offset, "offset", 0, "Offset for reports pagination")
	reportsCmd.Flags().BoolVar(&clear, "clear", false, "Clear all reports from database")
	rootCmd.AddCommand(reportsCmd)
}
