package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/orchestrator"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for any media file and find alternatives",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()
		client, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting client: %v\n", err)
			os.Exit(1)
		}

		query := strings.Join(args, " ")

		for {
			if query == "" {
				prompt := promptui.Prompt{
					Label: "Search for a movie or series",
				}
				var err error
				query, err = prompt.Run()
				if err != nil {
					return
				}
			}

			results, err := database.SearchMediaFiles(query, 50)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error searching: %v\n", err)
				os.Exit(1)
			}

			if len(results) == 0 {
				fmt.Printf("No results found for '%s'.\n", query)
				query = "" // Reset to prompt again
				continue
			}

			templates := &promptui.SelectTemplates{
				Label:    "{{ . }}",
				Active:   "\033[31m▶\033[0m {{ if or .IsExit .IsSearchAgain }}{{ .Title | yellow }}{{ else }}{{ .ArrInstance | faint }} | {{ .Quality | yellow }} | {{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Inactive: "  {{ if or .IsExit .IsSearchAgain }}{{ .Title | yellow }}{{ else }}{{ .ArrInstance | faint }} | {{ .Quality | yellow }} | {{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Selected: "\033[32m✔\033[0m {{ if .IsExit }}Exited{{ else if .IsSearchAgain }}New Search{{ else }}Selected: {{ .Title | cyan }}{{ end }}",
				Details: `
--------- File Details ---------
{{ if or .IsExit .IsSearchAgain }}
{{ .Title | faint }}
{{ else }}
{{ "Instance:" | faint }}	{{ .ArrInstance }} ({{ .ArrType }})
{{ "Path:" | faint }}	{{ .Path }}
{{ "Size:" | faint }}	{{ .Size }}
{{ "Quality:" | faint }}	{{ .Quality }}
{{ "Inode:" | faint }}	{{ .Inode }}
{{ "Torrents:" | faint }}
{{ .Torrents }}
{{ end }}`,
			}

			type searchItem struct {
				displayItem
				IsSearchAgain bool
			}

			items := make([]searchItem, len(results)+2)
			for i, r := range results {
				// Fetch torrents for this record
				torrentRecords, _ := database.GetTorrentsByInode(r.Inode)
				var torrentsInfo []string
				for _, t := range torrentRecords {
					addedStr := "unknown"
					if t.AddedAt > 0 {
						addedTime := time.Unix(t.AddedAt, 0)
						addedStr = addedTime.Format("2006-01-02 15:04")
					}
					torrentsInfo = append(torrentsInfo, fmt.Sprintf("  - [%s] %s (Added: %s)", t.ClientName, t.InfoHash[:8], addedStr))
				}
				torrentLine := strings.Join(torrentsInfo, "\n")
				if torrentLine == "" {
					torrentLine = "  No active torrents found in cache."
				}

				title := r.Title
				if r.ArrType == "sonarr" {
					filename := filepath.Base(r.Path)
					sxxexx := extractSXXEXX(filename)
					if sxxexx != "" {
						title = fmt.Sprintf("%s - %s", r.Title, sxxexx)
					} else {
						// Fallback to filename if parsing fails, but still keep series title for context
						title = fmt.Sprintf("%s - %s", r.Title, filename)
					}
				}

				items[i] = searchItem{
					displayItem: displayItem{
						Title:        title,
						Size:         humanize.Bytes(uint64(r.Size)),
						ArrInstance:  r.ArrInstance,
						ArrType:      r.ArrType,
						Path:         r.Path,
						Quality:      r.Quality,
						Torrents:     torrentLine,
						ID:           r.FileID,
						ItemID:       r.ItemID,
						SeasonNumber: r.SeasonNumber,
						Inode:        r.Inode,
						Record: db.CandidateRecord{
							MediaFileRecord: r,
						},
					},
				}
			}

			items[len(results)] = searchItem{
				displayItem:   displayItem{Title: "Search again..."},
				IsSearchAgain: true,
			}
			items[len(results)+1] = searchItem{
				displayItem: displayItem{Title: "Exit", IsExit: true},
			}

			prompt := promptui.Select{
				Label:     "Select a file to optimize",
				Items:     items,
				Templates: templates,
				Size:      10,
				Searcher: func(input string, index int) bool {
					item := items[index]
					name := strings.ToLower(item.Title + item.Path)
					return strings.Contains(name, strings.ToLower(input))
				},
			}

			index, _, err := prompt.Run()
			if err != nil {
				return
			}

			selected := items[index]
			if selected.IsExit {
				return
			}
			if selected.IsSearchAgain {
				query = ""
				continue
			}

			// Perform action
			orch := orchestrator.New(database, client, dryRun, verbose)
			if selected.ArrType == "radarr" {
				searchForRadarrAlternatives(selected.displayItem, database, orch, client)
			} else {
				searchForSonarrAlternatives(selected.displayItem, database, orch, client)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}

func extractSXXEXX(filename string) string {
	re := regexp.MustCompile(`(?i)s(\d+)e(\d+)`)
	match := re.FindString(filename)
	if match != "" {
		return strings.ToUpper(match)
	}
	// Try 1x01 format
	re2 := regexp.MustCompile(`(?i)(\d+)x(\d+)`)
	match2 := re2.FindString(filename)
	return strings.ToUpper(match2)
}
