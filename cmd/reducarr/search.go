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
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var showsMode bool

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search for any media file and find alternatives",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(database)
		client, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting client: %v\n", err)
			os.Exit(1)
		}

		if showsMode {
			runShowsSearch(database, client, args)
			return
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
	searchCmd.Flags().BoolVar(&showsMode, "shows", false, "Browse by series, season, and episode using Sonarr")
	rootCmd.AddCommand(searchCmd)
}

func runShowsSearch(database *db.DB, client *arrs.Client, args []string) {
	if len(client.Sonarr) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No Sonarr instance configured.")
		return
	}
	inst := client.Sonarr[0]
	query := strings.Join(args, " ")

	for {
		if query == "" {
			prompt := promptui.Prompt{
				Label: "Search for a series in Sonarr",
			}
			var err error
			query, err = prompt.Run()
			if err != nil {
				return
			}
		}

		spinner := ui.NewSpinner(fmt.Sprintf("Searching Sonarr for '%s'...", query))
		spinner.Start()
		ctx := context.Background()
		results, err := inst.LookupSeries(ctx, query)
		spinner.Stop()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error searching Sonarr: %v\n", err)
			return
		}

		var known []sonarr.SeriesResource
		for _, s := range results {
			if s.Id != nil && *s.Id > 0 {
				known = append(known, s)
			}
		}

		if len(known) == 0 {
			fmt.Printf("No series found in Sonarr for '%s'.\n", query)
			query = ""
			continue
		}

		seriesIdx, ok := pickSeries(known)
		if !ok {
			return
		}
		selectedSeries := known[seriesIdx]

		fullSeries, err := inst.GetSeriesByID(ctx, *selectedSeries.Id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching series details: %v\n", err)
			query = ""
			continue
		}

		for {
			seasonIdx, ok := pickSeason(fullSeries.Seasons)
			if !ok {
				break
			}
			selectedSeason := fullSeries.Seasons[seasonIdx]
			seasonNum := selectedSeason.GetSeasonNumber()

			files, err := database.GetMediaFilesBySeason(inst.Name(), *selectedSeries.Id, seasonNum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error querying DB: %v\n", err)
				continue
			}

			if len(files) == 0 {
				fmt.Printf("No files found in DB for S%02d of '%s' (run scan first).\n", seasonNum, arrs.GetString(selectedSeries.Title))
				continue
			}

			item, ok := pickEpisodeFile(files, database)
			if !ok {
				continue
			}

			orch := orchestrator.New(database, client, dryRun, verbose)
			searchForSonarrAlternatives(item, database, orch, client)
		}
	}
}

func pickSeries(series []sonarr.SeriesResource) (int, bool) {
	type seriesItem struct {
		Title  string
		Year   int32
		IsExit bool
	}

	items := make([]seriesItem, len(series)+1)
	for i, s := range series {
		year := int32(0)
		if s.Year != nil {
			year = *s.Year
		}
		items[i] = seriesItem{
			Title: arrs.GetString(s.Title),
			Year:  year,
		}
	}
	items[len(series)] = seriesItem{
		Title:  "Exit",
		IsExit: true,
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\033[31m▶\033[0m {{ if .IsExit }}{{ .Title | yellow }}{{ else }}{{ .Title | cyan }} ({{ .Year }}){{ end }}",
		Inactive: "  {{ if .IsExit }}{{ .Title | yellow }}{{ else }}{{ .Title | cyan }} ({{ .Year }}){{ end }}",
		Selected: "\033[32m✔\033[0m {{ if .IsExit }}Back{{ else }}Selected: {{ .Title | cyan }}{{ end }}",
	}

	prompt := promptui.Select{
		Label:     "Select a series",
		Items:     items,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := prompt.Run()
	if err != nil || items[idx].IsExit {
		return 0, false
	}
	return idx, true
}

func pickSeason(seasons []sonarr.SeasonResource) (int, bool) {
	type seasonItem struct {
		Title  string
		IsBack bool
	}

	items := make([]seasonItem, len(seasons)+1)
	for i, s := range seasons {
		num := s.GetSeasonNumber()
		title := fmt.Sprintf("Season %02d", num)
		if num == 0 {
			title = "Specials (S00)"
		}
		items[i] = seasonItem{
			Title: title,
		}
	}
	items[len(seasons)] = seasonItem{
		Title:  "Back",
		IsBack: true,
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\033[31m▶\033[0m {{ if .IsBack }}{{ .Title | yellow }}{{ else }}{{ .Title | cyan }}{{ end }}",
		Inactive: "  {{ if .IsBack }}{{ .Title | yellow }}{{ else }}{{ .Title | cyan }}{{ end }}",
		Selected: "\033[32m✔\033[0m {{ if .IsBack }}Back{{ else }}Selected: {{ .Title | cyan }}{{ end }}",
	}

	prompt := promptui.Select{
		Label:     "Select a season",
		Items:     items,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := prompt.Run()
	if err != nil || items[idx].IsBack {
		return 0, false
	}
	return idx, true
}

func pickEpisodeFile(results []db.MediaFileRecord, database *db.DB) (displayItem, bool) {
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\033[31m▶\033[0m {{ if .IsExit }}{{ .Title | yellow }}{{ else }}{{ .ArrInstance | faint }} | {{ .Quality | yellow }} | {{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
		Inactive: "  {{ if .IsExit }}{{ .Title | yellow }}{{ else }}{{ .ArrInstance | faint }} | {{ .Quality | yellow }} | {{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
		Selected: "\033[32m✔\033[0m {{ if .IsExit }}Back{{ else }}Selected: {{ .Title | cyan }}{{ end }}",
		Details: `
--------- File Details ---------
{{ if .IsExit }}
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

	items := make([]displayItem, len(results)+1)
	for i, r := range results {
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

		var title string
		filename := filepath.Base(r.Path)
		sxxexx := extractSXXEXX(filename)
		if sxxexx != "" {
			title = fmt.Sprintf("%s - %s", r.Title, sxxexx)
		} else {
			title = fmt.Sprintf("%s - %s", r.Title, filename)
		}

		items[i] = displayItem{
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
		}
	}

	items[len(results)] = displayItem{
		Title:  "Back",
		IsExit: true,
	}

	prompt := promptui.Select{
		Label:     "Select an episode file",
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
	if err != nil || items[index].IsExit {
		return displayItem{}, false
	}
	return items[index], true
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
