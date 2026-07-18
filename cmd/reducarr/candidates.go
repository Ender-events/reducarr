package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/orchestrator"
	"github.com/Ender-events/reducarr/internal/release"
	"github.com/Ender-events/reducarr/internal/sorting"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/dustin/go-humanize"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

type displayItem struct {
	Title        string
	Size         string
	ArrInstance  string
	ArrType      string
	Path         string
	Quality      string
	Reason       string
	Torrents     string
	ID           int32 // FileID
	ItemID       int32 // SeriesID or MovieID
	SeasonNumber int32
	Inode        uint64
	Record       db.CandidateRecord
	IsExit       bool
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
		client, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting client: %v\n", err)
			os.Exit(1)
		}

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
				Active:   "\033[31m▶\033[0m {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Inactive: "  {{ if .IsExit }}{{ .Title }}{{ else }}{{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Selected: "\033[32m✔\033[0m {{ if .IsExit }}Exited{{ else }}Candidate: {{ .Title | cyan }} ({{ .Size | green }}){{ end }}",
				Details: `
--------- Candidate Details ---------
{{ if .IsExit }}
{{ "Close the candidates browser" | faint }}
{{ else }}
{{ "Instance:" | faint }}	{{ .ArrInstance }} ({{ .ArrType }})
{{ "Path:" | faint }}	{{ .Path }}
{{ "Size:" | faint }}	{{ .Size }}
{{ "Quality:" | faint }}	{{ .Quality }}
{{ "Reason:" | faint }}	{{ .Reason | red }}
{{ "Inode:" | faint }}	{{ .Inode }}
{{ "Torrents:" | faint }}
{{ .Torrents }}
{{ end }}`,
			}

			items := make([]displayItem, len(candidates)+1)
			for i, c := range candidates {
				// Fetch torrents for this candidate
				torrentRecords, _ := database.GetTorrentsByInode(c.Inode)

				minSeedTime, _ := time.ParseDuration(cfg.Scoring.MinSeedDuration)

				var torrentsInfo []string
				for _, t := range torrentRecords {
					addedStr := "unknown"
					if t.AddedAt > 0 {
						addedTime := time.Unix(t.AddedAt, 0)
						addedStr = addedTime.Format("2006-01-02 15:04")

						// Check if age is less than minSeedTime
						if minSeedTime > 0 && time.Since(addedTime) < minSeedTime {
							// Orange color (ANSI 208 or simple yellow if not supported)
							addedStr = fmt.Sprintf("\033[38;5;208m%s\033[0m", addedStr)
						}
					}
					torrentsInfo = append(torrentsInfo, fmt.Sprintf("  - [%s] %s (Added: %s)", t.ClientName, t.InfoHash[:8], addedStr))
				}

				torrentLine := strings.Join(torrentsInfo, "\n")
				if torrentLine == "" {
					torrentLine = "  No active torrents found in cache."
				}

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
					Torrents:     torrentLine,
					ID:           c.FileID,
					ItemID:       c.ItemID,
					SeasonNumber: c.SeasonNumber,
					Inode:        c.Inode,
					Record:       c,
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
				Items: []string{"Search for Alternatives", "Delete (No replacement)", "Ignore", "Back", "Exit"},
			}

			_, action, err := actionPrompt.Run()
			if err != nil {
				if err == promptui.ErrInterrupt {
					continue
				}
				fmt.Fprintf(os.Stderr, "Action prompt failed: %v\n", err)
				return
			}

			orch := orchestrator.New(database, client, dryRun, verbose)

			switch action {
			case "Search for Alternatives":
				if selected.ArrType == "radarr" {
					searchForRadarrAlternatives(selected, database, orch, client)
				} else {
					searchForSonarrAlternatives(selected, database, orch, client)
				}
			case "Delete (No replacement)":
				confirm := promptui.Prompt{
					Label:     fmt.Sprintf("Are you sure you want to delete '%s' and ALL associated files/torrents?", selected.Title),
					IsConfirm: true,
				}
				if _, err := confirm.Run(); err == nil {
					if err := orch.DeleteCandidate(context.Background(), selected.Record); err != nil {
						fmt.Printf("\033[31m✘\033[0m Error deleting candidate: %v\n", err)
					} else {
						fmt.Println("\033[32m✔\033[0m Successfully deleted candidate and all associated files.")
					}
				}
			case "Ignore":
				if err := database.SetIgnoreCandidate(selected.ArrInstance, selected.ID, true); err != nil {
					fmt.Printf("\033[31m✘\033[0m Error ignoring candidate: %v\n", err)
				} else {
					fmt.Printf("\033[32m✔\033[0m Candidate '%s' will now be ignored in future scans.\n", selected.Title)
				}
			case "Back":
				continue
			case "Exit":
				return
			}
		}
	},
}

func showTorrentContext(item displayItem, database *db.DB) {
	torrentRecords, _ := database.GetTorrentsByInode(item.Inode)
	if len(torrentRecords) > 0 {
		fmt.Printf("\n--- Files in current Torrent ---\n")
		seenFiles := make(map[string]bool)
		for _, t := range torrentRecords {
			allFiles, _ := database.GetTorrentsByHash(t.InfoHash)
			for _, tf := range allFiles {
				if seenFiles[tf.FilePath] {
					continue
				}
				seenFiles[tf.FilePath] = true

				status := "\033[33m[Unknown]\033[0m"
				m, _ := database.GetMediaFileByInode(tf.Inode)
				if m != nil {
					if isCandidate(m.FileID, m.ArrInstance, database) {
						status = fmt.Sprintf("\033[31m[%d]\033[0m", m.FileID)
					} else {
						status = fmt.Sprintf("\033[32m[%d]\033[0m", m.FileID)
					}
				}
				fmt.Printf("  %s %s\n", status, tf.FilePath)
			}
		}
		fmt.Println()
	}
}

func sortAndSelectRelease(releases []release.Release, item displayItem, database *db.DB) any {
	if len(releases) == 0 {
		fmt.Println("No releases found.")
		return nil
	}

	// 1. Sort using release engine
	sorting.Sort(releases)

	// 2. Templates
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "\033[31m▶\033[0m {{ .Title | cyan }} [{{ .ScoreStr | magenta }}] | {{ .SizeStr | yellow }} | {{ .Indexer | green }}",
		Inactive: "  {{ .Title | cyan }} [{{ .ScoreStr | magenta }}] | {{ .SizeStr | yellow }} | {{ .Indexer | green }}",
		Selected: "\033[32m✔\033[0m Selected: {{ .Title | cyan }}",
		Details: `
--------- Release Details ---------
{{ "Indexer:" | faint }}   {{ .Indexer }}
{{ "Score:" | faint }}     {{ .ScoreStr }}
{{ "Size:" | faint }}      {{ .SizeStr }}
{{ "Seeders:" | faint }}   {{ .Seeders }}
{{ "Quality:" | faint }}   {{ .Quality }}
{{ "Protocol:" | faint }}  {{ .Protocol }}
{{ if .RejectionsStr }}{{ "REJECTED:" | red }}  {{ .RejectionsStr }}{{ end }}`,
	}

	type displayRelease struct {
		Title         string
		SizeStr       string
		Indexer       string
		Seeders       int32
		Quality       string
		Protocol      string
		RejectionsStr string
		ScoreStr      string
		Raw           any
	}

	displayItems := make([]displayRelease, len(releases))
	for i, r := range releases {
		scoreStr := "n/a"
		if r.Score != nil {
			scoreStr = fmt.Sprintf("%d", *r.Score)
		}

		displayItems[i] = displayRelease{
			Title:         r.Title,
			SizeStr:       humanize.Bytes(uint64(r.Size)),
			Indexer:       r.Indexer,
			Seeders:       r.Seeders,
			Quality:       r.Quality,
			Protocol:      r.Protocol,
			RejectionsStr: strings.Join(r.Rejections, ", "),
			ScoreStr:      scoreStr,
			Raw:           r.Raw,
		}
	}

	prompt := promptui.Select{
		Label:     "Select a release to grab",
		Items:     displayItems,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := prompt.Run()
	if err != nil {
		return nil
	}

	selected := displayItems[idx]

	// 3. Multi-file Warning (Common logic)
	torrentRecords, _ := database.GetTorrentsByInode(item.Inode)
	nonCandidates := 0
	for _, t := range torrentRecords {
		allFiles, _ := database.GetTorrentsByHash(t.InfoHash)
		for _, tf := range allFiles {
			m, _ := database.GetMediaFileByInode(tf.Inode)
			if m != nil && !isCandidate(m.FileID, m.ArrInstance, database) {
				nonCandidates++
			}
		}
	}

	if nonCandidates > 0 {
		fmt.Printf("\n\033[33m⚠ WARNING\033[0m: This torrent contains %d files that are already optimized.\n", nonCandidates)
		fmt.Println("Replacing this torrent will replace them anyway.")
	}

	// 4. Confirm
	confirmPrompt := promptui.Prompt{
		Label:     fmt.Sprintf("Grab '%s' and replace current files?", selected.Title),
		IsConfirm: true,
	}

	_, err = confirmPrompt.Run()
	if err != nil {
		return nil
	}

	return selected.Raw
}

func searchForRadarrAlternatives(item displayItem, database *db.DB, orch *orchestrator.Orchestrator, client *arrs.Client) {
	inst := client.FindRadarr(item.ArrInstance)
	if inst == nil {
		fmt.Printf("Error: could not find Radarr instance %s\n", item.ArrInstance)
		return
	}

	showTorrentContext(item, database)

	spinner := ui.NewSpinner(fmt.Sprintf("Searching for releases for: %s...", item.Title))
	spinner.Start()

	ctx := context.Background()
	_ = inst.TriggerMovieSearch(ctx, item.ItemID)

	rawReleases, err := inst.ListReleases(ctx, item.ItemID)
	spinner.Stop()

	if err != nil {
		fmt.Printf("Error fetching releases: %v\n", err)
		return
	}

	releases := make([]release.Release, len(rawReleases))
	for i, r := range rawReleases {
		quality := ""
		if r.Quality != nil && r.Quality.Quality != nil {
			quality = arrs.GetStringRadarr(r.Quality.Quality.Name)
		}

		seeders := int32(0)
		if r.Seeders.Get() != nil {
			seeders = *r.Seeders.Get()
		}

		protocol := "unknown"
		if r.Protocol != nil {
			protocol = string(*r.Protocol)
		}

		releases[i] = release.Release{
			Title:      arrs.GetStringRadarr(r.Title),
			Size:       *r.Size,
			Indexer:    arrs.GetStringRadarr(r.Indexer),
			Seeders:    seeders,
			Quality:    quality,
			Protocol:   protocol,
			Rejections: r.Rejections,
			Score:      r.CustomFormatScore,
			Raw:        &rawReleases[i],
		}
	}

	selectedRaw := sortAndSelectRelease(releases, item, database)
	if selectedRaw == nil {
		return
	}

	selected := selectedRaw.(*radarr.ReleaseResource)

	if err := orch.UpgradeCandidate(ctx, item.Record, selected); err != nil {
		fmt.Printf("\033[31m✘\033[0m Error during upgrade: %v\n", err)
		return
	}

	fmt.Println("\033[32m✔\033[0m Successfully triggered upgrade in Radarr.")
}

func searchForSonarrAlternatives(item displayItem, database *db.DB, orch *orchestrator.Orchestrator, client *arrs.Client) {
	inst := client.FindSonarr(item.ArrInstance)
	if inst == nil {
		fmt.Printf("Error: could not find Sonarr instance %s\n", item.ArrInstance)
		return
	}

	showTorrentContext(item, database)

	ctx := context.Background()

	scopePrompt := promptui.Select{
		Label: "Select search scope",
		Items: []string{
			"Search each episode individually",
			fmt.Sprintf("Search complete season (S%02d)", item.SeasonNumber),
			"Search entire series",
			"Back",
		},
	}

	scopeIdx, scope, err := scopePrompt.Run()
	if err != nil || scope == "Back" {
		return
	}

	spinner := ui.NewSpinner(fmt.Sprintf("Searching for releases (%s)...", scope))
	spinner.Start()

	var rawReleases []sonarr.ReleaseResource
	var fetchErr error

	switch scopeIdx {
	case 0: // Individual
		episodes, err := inst.ListEpisodes(ctx, item.ItemID)
		if err != nil {
			fetchErr = fmt.Errorf("list episodes: %w", err)
		} else {
			var episodeID int32
			for _, ep := range episodes {
				if ep.EpisodeFileId != nil && *ep.EpisodeFileId == item.ID {
					if ep.Id != nil {
						episodeID = *ep.Id
						break
					}
				}
			}
			if episodeID == 0 {
				fetchErr = fmt.Errorf("could not find episode associated with file ID %d", item.ID)
			} else {
				rawReleases, fetchErr = inst.ListReleases(ctx, &episodeID, nil, nil)
			}
		}
	case 1: // Season
		rawReleases, fetchErr = inst.ListReleases(ctx, nil, &item.ItemID, &item.SeasonNumber)
	case 2: // Series
		rawReleases, fetchErr = inst.ListReleases(ctx, nil, &item.ItemID, nil)
	}

	spinner.Stop()

	if fetchErr != nil {
		fmt.Printf("Error fetching releases: %v\n", fetchErr)
		return
	}

	releases := make([]release.Release, len(rawReleases))
	for i, r := range rawReleases {
		quality := ""
		if r.Quality != nil && r.Quality.Quality != nil {
			quality = arrs.GetString(r.Quality.Quality.Name)
		}

		seeders := int32(0)
		if r.Seeders.Get() != nil {
			seeders = *r.Seeders.Get()
		}

		protocol := "unknown"
		if r.Protocol != nil {
			protocol = string(*r.Protocol)
		}

		releases[i] = release.Release{
			Title:      arrs.GetString(r.Title),
			Size:       *r.Size,
			Indexer:    arrs.GetString(r.Indexer),
			Seeders:    seeders,
			Quality:    quality,
			Protocol:   protocol,
			Rejections: r.Rejections,
			Score:      r.CustomFormatScore,
			Raw:        &rawReleases[i],
		}
	}

	selectedRaw := sortAndSelectRelease(releases, item, database)
	if selectedRaw == nil {
		return
	}

	selected := selectedRaw.(*sonarr.ReleaseResource)

	if err := orch.UpgradeCandidate(ctx, item.Record, selected); err != nil {
		fmt.Printf("\033[31m✘\033[0m Error during upgrade: %v\n", err)
		return
	}

	fmt.Println("\033[32m✔\033[0m Successfully triggered upgrade in Sonarr.")
}

func isCandidate(fileID int32, instance string, database *db.DB) bool {
	var exists bool
	_ = database.QueryRow("SELECT EXISTS(SELECT 1 FROM candidates WHERE file_id = ? AND arr_instance = ?)", fileID, instance).Scan(&exists)
	return exists
}

func init() {
	candidatesCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Do not perform any destructive actions (torrent deletion, release grab)")
	rootCmd.AddCommand(candidatesCmd)
}
