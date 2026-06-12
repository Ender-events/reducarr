package web

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
)

func NewRouter(database *db.DB, client *arrs.Client, expectedUser, expectedPass string) http.Handler {
	mux := http.NewServeMux()

	// Health check (unauthenticated)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	// Login page
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		LoginPage("").Render(r.Context(), w)
	})

	// Login action
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		user := r.FormValue("username")
		pass := r.FormValue("password")

		if user != expectedUser || pass != expectedPass {
			LoginPage("Invalid username or password").Render(r.Context(), w)
			return
		}

		// Create session
		token := GenerateToken()
		expiresAt := time.Now().Add(24 * 7 * time.Hour) // 24 hours * 7 days
		if err := database.CreateSession(token, user, expiresAt); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set cookie
		SetSessionCookie(w, token, expiresAt)

		// Redirect to home
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// Logout action
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("reducarr_session")
		if err == nil {
			_ = database.DeleteSession(cookie.Value)
		}
		ClearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Main dashboard (protected)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, "<h1>Reducarr Dashboard</h1><p>Authenticated as %s. UI implementation in progress.</p>", expectedUser)
		fmt.Fprintf(w, `<form action="/logout" method="POST"><button type="submit">Logout</button></form>`)
	})

	// Wrap mux with session authentication middleware
	return SessionAuth(database)(mux)
}
