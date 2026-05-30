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

var candidatesCmd = &cobra.Command{
	Use:   "candidates",
	Short: "Browse and manage optimization candidates interactively",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		for {
			candidates, err := database.GetCandidatesWithMedia()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error fetching candidates: %v\n", err)
				os.Exit(1)
			}

			if len(candidates) == 0 {
				fmt.Println("No candidates found. Run 'reducarr scan' first.")
				return
			}

			templates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\033[31m▶\033[0m {{ .Title | cyan }} ({{ .Size | yellow }})",
				Inactive: "  {{ .Title | cyan }} ({{ .Size | yellow }})",
				Selected: "\033[32m✔\033[0m Selected: {{ .Title | cyan }}",
				Details: `
--------- Candidate Details ---------
{{ "Instance:" | faint }}	{{ .ArrInstance }} ({{ .ArrType }})
{{ "Path:" | faint }}	{{ .Path }}
{{ "Size:" | faint }}	{{ .Size }}
{{ "Quality:" | faint }}	{{ .Quality }}
{{ "Reason:" | faint }}	{{ .Reason | red }}
{{ "Note:" | faint }}	Use 'reducarr inspect' for full torrent details.`,
			}

			type displayItem struct {
				Title       string
				Size        string
				ArrInstance string
				ArrType     string
				Path        string
				Quality     string
				Reason      string
				ID          int32
				IsExit      bool
			}

			items := make([]displayItem, len(candidates)+1)
			for i, c := range candidates {
				items[i] = displayItem{
					Title:       c.Title,
					Size:        humanize.Bytes(uint64(c.Size)),
					ArrInstance: c.ArrInstance,
					ArrType:     c.ArrType,
					Path:        c.Path,
					Quality:     c.Quality,
					Reason:      c.Reason,
					ID:          c.FileID,
				}
			}
			items[len(candidates)] = displayItem{
				Title:  "\033[33mExit\033[0m",
				IsExit: true,
			}

			searcher := func(input string, index int) bool {
				item := items[index]
				if item.IsExit {
					return strings.Contains("exit", strings.ToLower(input))
				}
				name := strings.ToLower(item.Title + item.Path)
				input = strings.ToLower(input)
				return strings.Contains(name, input)
			}

			prompt := promptui.Select{
				Label:     "Select a candidate to optimize",
				Items:     items,
				Templates: templates,
				Size:      10,
				Searcher:  searcher,
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

			// Action Menu
			actionPrompt := promptui.Select{
				Label: fmt.Sprintf("Action for: %s", selected.Title),
				Items: []string{"Search for Alternatives", "Ignore", "Back", "Exit"},
			}

			_, action, err := actionPrompt.Run()
			if err != nil {
				if err == promptui.ErrInterrupt {
					continue
				}
				fmt.Fprintf(os.Stderr, "Action prompt failed: %v\n", err)
				return
			}

			switch action {
			case "Search for Alternatives":
				fmt.Printf("Searching for alternatives for: %s (Not yet implemented)\n", selected.Title)
			case "Ignore":
				fmt.Printf("Ignoring: %s (Not yet implemented)\n", selected.Title)
			case "Back":
				continue
			case "Exit":
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(candidatesCmd)
}
