package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type displayItem struct {
	Title       string
	Size        string
	ArrInstance string
	ArrType     string
	Path        string
	Quality     string
	Reason      string
	Torrents    string
	ID          int32
	ItemID      int32
	IsExit      bool
}

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
{{ "Torrents:" | faint }}
{{ .Torrents }}`,
			}

			items := make([]displayItem, len(candidates)+1)
			for i, c := range candidates {
				// Fetch torrents for this candidate
				torrentRecords, _ := database.GetTorrentsByInode(c.Inode)
				var torrentsInfo []string
				for _, t := range torrentRecords {
					addedStr := "unknown"
					if t.AddedAt > 0 {
						addedStr = time.Unix(t.AddedAt, 0).Format("2006-01-02 15:04")
					}
					torrentsInfo = append(torrentsInfo, fmt.Sprintf("  - [%s] %s (Added: %s)", t.ClientName, t.InfoHash[:8], addedStr))
				}

				torrentLine := strings.Join(torrentsInfo, "\n")
				if torrentLine == "" {
					torrentLine = "  No active torrents found in cache."
				}

				items[i] = displayItem{
					Title:       c.Title,
					Size:        humanize.Bytes(uint64(c.Size)),
					ArrInstance: c.ArrInstance,
					ArrType:     c.ArrType,
					Path:        c.Path,
					Quality:     c.Quality,
					Reason:      c.Reason,
					Torrents:    torrentLine,
					ID:          c.FileID,
					ItemID:      c.ItemID,
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
				if selected.ArrType == "radarr" {
					searchForRadarrAlternatives(selected, database)
				} else {
					fmt.Println("Interactive search for Sonarr is not yet implemented.")
				}
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

func searchForRadarrAlternatives(item displayItem, database *db.DB) {
	// 1. Setup Client
	sonarrInstances := make([]arrs.ArrInstance, len(cfg.Sonarr))
	for i, s := range cfg.Sonarr {
		sonarrInstances[i] = arrs.ArrInstance{Name: s.Name, URL: s.URL, APIKey: s.APIKey, PathMappings: s.PathMappings}
	}
	radarrInstances := make([]arrs.ArrInstance, len(cfg.Radarr))
	for i, r := range cfg.Radarr {
		radarrInstances[i] = arrs.ArrInstance{Name: r.Name, URL: r.URL, APIKey: r.APIKey, PathMappings: r.PathMappings}
	}
	client := arrs.NewClient(sonarrInstances, radarrInstances, nil)

	inst := client.FindRadarr(item.ArrInstance)
	if inst == nil {
		fmt.Printf("Error: could not find Radarr instance %s\n", item.ArrInstance)
		return
	}

	// 2. Trigger Search
	spinner := ui.NewSpinner(fmt.Sprintf("Searching for releases for: %s...", item.Title))
	spinner.Start()

	ctx := context.Background()
	_ = inst.TriggerMovieSearch(ctx, item.ItemID) // Trigger refresh

	releases, err := inst.ListReleases(ctx, item.ItemID)
	spinner.Stop()

	if err != nil {
		fmt.Printf("Error fetching releases: %v\n", err)
		return
	}

	if len(releases) == 0 {
		fmt.Println("No releases found.")
		return
	}

	// 3. Improved Sort
	sort.Slice(releases, func(i, j int) bool {
		// Priority 1: Rejection Status (Approved first)
		iApproved := len(releases[i].Rejections) == 0
		jApproved := len(releases[j].Rejections) == 0
		if iApproved != jApproved {
			return iApproved // True (Approved) comes first
		}

		// Priority 2: CustomFormatScore (Higher is better)
		ci := getScore(releases[i].CustomFormatScore)
		cj := getScore(releases[j].CustomFormatScore)
		if ci != cj {
			return ci > cj
		}

		// Priority 3: Rejection Severity (Lower is better)
		si := getRejectionSeverity(releases[i].Rejections)
		sj := getRejectionSeverity(releases[j].Rejections)
		if si != sj {
			return si < sj
		}

		// Priority 4: Size (Smaller is better)
		return *releases[i].Size < *releases[j].Size
	})

	// 4. Selection Menu
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\033[31m▶\033[0m {{ .Title | cyan }} [{{ .Score | magenta }}] | {{ .SizeStr | yellow }} | {{ .Indexer | green }}",
		Inactive: "  {{ .Title | cyan }} [{{ .Score | magenta }}] | {{ .SizeStr | yellow }} | {{ .Indexer | green }}",
		Details: `
--------- Release Details ---------
{{ "Indexer:" | faint }}	{{ .Indexer }}
{{ "Score:" | faint }}	{{ .Score }}
{{ "Size:" | faint }}	{{ .SizeStr }}
{{ "Seeders:" | faint }}	{{ .Seeders }}
{{ "Quality:" | faint }}	{{ .Quality }}
{{ "Protocol:" | faint }}	{{ .Protocol }}
{{ if .Rejections }}{{ "REJECTED:" | red }}	{{ .Rejections }}{{ end }}`,
	}

	type releaseItem struct {
		Title      string
		SizeStr    string
		Indexer    string
		Seeders    string
		Quality    string
		Protocol   string
		Rejections string
		Score      string
		Raw        *radarr.ReleaseResource
	}

	displayReleases := make([]releaseItem, len(releases))
	for i, r := range releases {
		quality := ""
		if r.Quality != nil && r.Quality.Quality != nil {
			quality = arrs.GetStringRadarr(r.Quality.Quality.Name)
		}

		indexer := arrs.GetStringRadarr(r.Indexer)
		title := arrs.GetStringRadarr(r.Title)

		seeders := "0"
		if r.Seeders.Get() != nil {
			seeders = fmt.Sprintf("%d", *r.Seeders.Get())
		}

		protocol := "unknown"
		if r.Protocol != nil {
			protocol = string(*r.Protocol)
		}

		scoreStr := "no value"
		if r.CustomFormatScore != nil {
			scoreStr = fmt.Sprintf("%d", *r.CustomFormatScore)
		}

		displayReleases[i] = releaseItem{
			Title:      title,
			SizeStr:    humanize.Bytes(uint64(*r.Size)),
			Indexer:    indexer,
			Seeders:    seeders,
			Quality:    quality,
			Protocol:   protocol,
			Rejections: strings.Join(r.Rejections, ", "),
			Score:      scoreStr,
			Raw:        &releases[i],
		}
	}

	prompt := promptui.Select{
		Label:     "Select a release to grab",
		Items:     displayReleases,
		Templates: templates,
		Size:      10,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return
	}

	selected := displayReleases[index]

	// 5. Confirm and Grab
	confirmPrompt := promptui.Prompt{
		Label:     fmt.Sprintf("Grab '%s' and replace current file?", selected.Title),
		IsConfirm: true,
	}

	_, err = confirmPrompt.Run()
	if err != nil {
		return
	}

	// TODO: Phase 4 Deletion logic would go here.
	// For now, we just trigger the grab.

	fmt.Printf("Grabbing release: %s...\n", selected.Title)
	err = inst.DownloadRelease(ctx, selected.Raw)
	if err != nil {
		fmt.Printf("Error grabbing release: %v\n", err)
		return
	}

	fmt.Println("\033[32m✔\033[0m Successfully triggered grab in Radarr.")
}

func getScore(s *int32) int32 {
	if s == nil {
		return math.MinInt32
	}
	return *s
}

func getRejectionSeverity(rejections []string) int {
	if len(rejections) == 0 {
		return 0 // Approved
	}

	hasGeneral := false
	for _, r := range rejections {
		if strings.Contains(r, "Unknown Movie") {
			return 3 // Absolute worst
		}
		if !strings.Contains(r, "Quality profile does not allow upgrades") {
			hasGeneral = true
		}
	}

	if hasGeneral {
		return 2
	}
	return 1
}

func init() {
	rootCmd.AddCommand(candidatesCmd)
}
