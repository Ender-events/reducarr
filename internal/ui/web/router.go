package web

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/orchestrator"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
)

func NewRouter(database *db.DB, client *arrs.Client, expectedUser, expectedPass string, verbose bool) http.Handler {
	mux := http.NewServeMux()

	// Logger helper
	vlog := func(format string, v ...any) {
		if verbose {
			log.Printf("[WEB] "+format, v...)
		}
	}

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	// Login page
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("reducarr_session")
		if err == nil {
			_, err = database.GetSession(cookie.Value)
			if err == nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
		LoginPage("").Render(r.Context(), w)
	})

	// Login action
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		user := r.FormValue("username")
		pass := r.FormValue("password")

		dbPass, _ := database.GetUser(user)
		targetPass := expectedPass
		if dbPass != "" {
			targetPass = dbPass
		}

		if user != expectedUser || pass != targetPass {
			vlog("Failed login attempt for user: %s", user)
			LoginPage("Invalid username or password").Render(r.Context(), w)
			return
		}

		token := GenerateToken()
		expiresAt := time.Now().Add(24 * 7 * time.Hour)
		if err := database.CreateSession(token, user, expiresAt); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		vlog("User logged in: %s", user)
		SetSessionCookie(w, token, expiresAt)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// Logout action
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("reducarr_session")
		if err == nil {
			vlog("User logging out: %s", expectedUser)
			_ = database.DeleteSession(cookie.Value)
		}
		ClearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Dashboard
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		vlog("Accessing Dashboard")
		stats, err := database.GetDashboardStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		webStats := DashboardStats{
			TotalSpaceSaved:   stats.TotalSpaceSaved,
			PendingCandidates: stats.PendingCandidates,
			IgnoredFiles:      stats.IgnoredFiles,
			FailedActions:     stats.FailedActions,
		}
		IndexPage(expectedUser, webStats).Render(r.Context(), w)
	})

	// Candidates
	mux.HandleFunc("GET /candidates", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Candidates page")
		candidates, err := database.GetCandidatesWithMedia()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		CandidatesPage(expectedUser, candidates).Render(r.Context(), w)
	})

	// Reports
	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Reports page")
		reports, err := database.GetReports(100, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ReportsPage(expectedUser, reports).Render(r.Context(), w)
	})

	// Search
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Search page")
		SearchPage(expectedUser).Render(r.Context(), w)
	})

	// --- API Endpoints for HTMX ---

	// Manual Search API
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if len(query) < 2 {
			return
		}
		vlog("Searching library for: %s", query)
		results, err := database.SearchMediaFiles(query, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		SearchResults(expectedUser, results).Render(r.Context(), w)
	})

	// Profile Modal
	mux.HandleFunc("GET /api/user/password", func(w http.ResponseWriter, r *http.Request) {
		vlog("Opening profile modal")
		ChangePasswordModal(expectedUser, "", false).Render(r.Context(), w)
	})

	// Change Password Action
	mux.HandleFunc("POST /api/user/password", func(w http.ResponseWriter, r *http.Request) {
		vlog("Updating password for user: %s", expectedUser)
		pass := r.FormValue("password")
		confirm := r.FormValue("confirm")

		if pass != confirm {
			ChangePasswordModal(expectedUser, "Passwords do not match.", false).Render(r.Context(), w)
			return
		}

		if len(pass) < 8 {
			ChangePasswordModal(expectedUser, "Password must be at least 8 characters.", false).Render(r.Context(), w)
			return
		}

		if err := database.UpsertUser(expectedUser, pass); err != nil {
			ChangePasswordModal(expectedUser, "Failed to update password in database.", false).Render(r.Context(), w)
			return
		}

		ChangePasswordModal(expectedUser, "", true).Render(r.Context(), w)
	})

	// Ignore Candidate
	mux.HandleFunc("POST /api/candidates/{instance}/{id}/ignore", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Ignoring candidate: %s:%d", instance, id)
		if err := database.SetIgnoreCandidate(instance, id, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Delete Candidate
	mux.HandleFunc("DELETE /api/candidates/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Deleting candidate: %s:%d", instance, id)
		candidates, _ := database.GetCandidatesWithMedia()
		var target *db.CandidateRecord
		for _, c := range candidates {
			if c.ArrInstance == instance && c.FileID == id {
				target = &c
				break
			}
		}

		if target == nil {
			http.Error(w, "Candidate not found", http.StatusNotFound)
			return
		}

		orch := orchestrator.New(database, client, false, verbose)
		if err := orch.DeleteCandidate(r.Context(), *target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Search Alternatives
	mux.HandleFunc("GET /api/candidates/{instance}/{id}/search", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Searching alternatives for: %s:%d", instance, id)

		var target db.CandidateRecord
		found := false
		candidates, _ := database.GetCandidatesWithMedia()
		for _, c := range candidates {
			if c.ArrInstance == instance && c.FileID == id {
				target = c
				found = true
				break
			}
		}

		if !found {
			media, _ := database.SearchMediaFiles("", 10000)
			for _, m := range media {
				if m.ArrInstance == instance && m.FileID == id {
					target = db.CandidateRecord{MediaFileRecord: m}
					found = true
					break
				}
			}
		}

		if !found {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		var releaseInfos []ReleaseInfo
		if target.ArrType == "radarr" {
			inst := client.FindRadarr(instance)
			_ = inst.TriggerMovieSearch(r.Context(), target.ItemID)
			releases, _ := inst.ListReleases(r.Context(), target.ItemID)
			for _, rl := range releases {
				score := int32(0)
				if rl.CustomFormatScore != nil {
					score = *rl.CustomFormatScore
				}
				seeders := int32(0)
				if rl.Seeders.Get() != nil {
					seeders = *rl.Seeders.Get()
				}
				releaseInfos = append(releaseInfos, ReleaseInfo{
					GUID:       arrs.GetStringRadarr(rl.Guid),
					Title:      arrs.GetStringRadarr(rl.Title),
					Size:       *rl.Size,
					Indexer:    arrs.GetStringRadarr(rl.Indexer),
					Seeders:    seeders,
					Quality:    arrs.GetStringRadarr(rl.Quality.Quality.Name),
					Score:      score,
					Rejections: rl.Rejections,
				})
			}
		} else {
			inst := client.FindSonarr(instance)
			episodes, _ := inst.ListEpisodes(r.Context(), target.ItemID)
			var epID int32
			for _, ep := range episodes {
				if ep.EpisodeFileId != nil && *ep.EpisodeFileId == target.FileID {
					epID = *ep.Id
					break
				}
			}
			if epID != 0 {
				releases, _ := inst.ListReleases(r.Context(), &epID, nil, nil)
				for _, rl := range releases {
					score := int32(0)
					if rl.CustomFormatScore != nil {
						score = *rl.CustomFormatScore
					}
					seeders := int32(0)
					if rl.Seeders.Get() != nil {
						seeders = *rl.Seeders.Get()
					}
					releaseInfos = append(releaseInfos, ReleaseInfo{
						GUID:       arrs.GetString(rl.Guid),
						Title:      arrs.GetString(rl.Title),
						Size:       *rl.Size,
						Indexer:    arrs.GetString(rl.Indexer),
						Seeders:    seeders,
						Quality:    arrs.GetString(rl.Quality.Quality.Name),
						Score:      score,
						Rejections: rl.Rejections,
					})
				}
			}
		}

		vlog("Found %d releases for %s", len(releaseInfos), target.Title)
		SearchModal(target.Title, instance, id, releaseInfos).Render(r.Context(), w)
	})

	// Grab Release
	mux.HandleFunc("POST /api/candidates/{instance}/{id}/grab", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)
		guid := r.FormValue("guid")

		var target db.CandidateRecord
		found := false
		candidates, _ := database.GetCandidatesWithMedia()
		for _, c := range candidates {
			if c.ArrInstance == instance && c.FileID == id {
				target = c
				found = true
				break
			}
		}
		if !found {
			media, _ := database.SearchMediaFiles("", 10000)
			for _, m := range media {
				if m.ArrInstance == instance && m.FileID == id {
					target = db.CandidateRecord{MediaFileRecord: m}
					found = true
					break
				}
			}
		}

		if !found || guid == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		vlog("Grabbing release with GUID %s for %s", guid, target.Title)
		orch := orchestrator.New(database, client, false, verbose)
		var err error
		if target.ArrType == "radarr" {
			inst := client.FindRadarr(instance)
			releases, _ := inst.ListReleases(r.Context(), target.ItemID)
			var selected *radarr.ReleaseResource
			for i := range releases {
				if arrs.GetStringRadarr(releases[i].Guid) == guid {
					selected = &releases[i]
					break
				}
			}
			if selected != nil {
				err = orch.UpgradeCandidate(r.Context(), target, selected)
			}
		} else {
			inst := client.FindSonarr(instance)
			episodes, _ := inst.ListEpisodes(r.Context(), target.ItemID)
			var epID int32
			for _, ep := range episodes {
				if ep.EpisodeFileId != nil && *ep.EpisodeFileId == target.FileID {
					epID = *ep.Id
					break
				}
			}
			releases, _ := inst.ListReleases(r.Context(), &epID, nil, nil)
			var selected *sonarr.ReleaseResource
			for i := range releases {
				if arrs.GetString(releases[i].Guid) == guid {
					selected = &releases[i]
					break
				}
			}
			if selected != nil {
				err = orch.UpgradeCandidate(r.Context(), target, selected)
			}
		}

		if err != nil {
			vlog("ERROR grabbing release: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		vlog("Successfully triggered upgrade for: %s", target.Title)
		w.WriteHeader(http.StatusOK)
	})

	return SessionAuth(database)(mux)
}
