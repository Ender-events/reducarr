package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var ignoredCmd = &cobra.Command{
	Use:   "ignored",
	Short: "Manage ignored candidates",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(database)

		for {
			ignored, err := database.GetIgnoredCandidates()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching ignored candidates: %v\n", err)
				os.Exit(1)
			}

			if len(ignored) == 0 {
				fmt.Println("No ignored candidates found.")
				return
			}

			templates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\033[31m▶\033[0m {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Inactive: "  {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Selected: "\033[32m✔\033[0m {{ if .IsExit }}Exited{{ else }}Ignored: {{ .Title | cyan }}{{ end }}",
				Details: `
--------- Ignored Details ---------
{{ if .IsExit }}
{{ "Close the ignored browser" | faint }}
{{ else }}
{{ "Instance:" | faint }}	{{ .ArrInstance }} ({{ .ArrType }})
{{ "Path:" | faint }}	{{ .Path }}
{{ "Size:" | faint }}	{{ .Size }}
{{ "Quality:" | faint }}	{{ .Quality }}
{{ "Reason:" | faint }}	{{ .Reason | red }}
{{ "Inode:" | faint }}	{{ .Inode }}
{{ end }}`,
			}

			items := make([]displayItem, len(ignored)+1)
			for i, c := range ignored {
				title := c.Title
				if c.ArrType == "sonarr" && c.SeasonNumber > 0 {
					title = fmt.Sprintf("%s - S%02d", c.Title, c.SeasonNumber)
				}
				items[i] = displayItem{
					Title:        title,
					Size:         humanize.Bytes(uint64(c.Size)),
					ArrInstance:  c.ArrInstance,
					ArrType:      c.ArrType,
					Path:         c.Path,
					Quality:      c.Quality,
					Reason:       c.Reason,
					ID:           c.FileID,
					ItemID:       c.ItemID,
					SeasonNumber: c.SeasonNumber,
					Inode:        c.Inode,
					Record:       c,
				}
			}
			items[len(ignored)] = displayItem{
				Title:  "\033[33mExit\033[0m",
				IsExit: true,
			}

			prompt := promptui.Select{
				Label:     "Select an ignored candidate to manage",
				Items:     items,
				Templates: templates,
				Size:      10,
				Searcher: func(input string, index int) bool {
					item := items[index]
					if item.IsExit {
						return strings.Contains("exit", strings.ToLower(input))
					}
					name := strings.ToLower(item.Title + item.Path)
					return strings.Contains(name, strings.ToLower(input))
				},
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

			// Sub-menu for action
			actionPrompt := promptui.Select{
				Label: fmt.Sprintf("Action for: %s", selected.Title),
				Items: []string{"Remove from Ignore list", "Back", "Exit"},
			}

			_, action, err := actionPrompt.Run()
			if err != nil {
				if err == promptui.ErrInterrupt {
					continue
				}
				return
			}

			switch action {
			case "Remove from Ignore list":
				if err := database.RemoveIgnore(selected.ArrInstance, selected.ID); err != nil {
					fmt.Printf("\033[31m✘\033[0m Error removing from ignore list: %v\n", err)
				} else {
					fmt.Printf("\033[32m✔\033[0m Candidate '%s' is no longer ignored.\n", selected.Title)
				}
			case "Back":
				continue
			case "Exit":
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(ignoredCmd)
}
