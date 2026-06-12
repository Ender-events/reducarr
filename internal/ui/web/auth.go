package web

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
	"net/http"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.?!_+*-"

func GenerateRandomPassword(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[num.Int64()]
	}
	return string(result), nil
}

func GenerateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func SessionAuth(database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for login and health
			if r.URL.Path == "/login" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie("reducarr_session")
			if err != nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			_, err = database.GetSession(cookie.Value)
			if err != nil {
				// Invalid or expired session
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func SetSessionCookie(w http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "reducarr_session",
		Value:    token,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   false, // Set to true if using HTTPS
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "reducarr_session",
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Path:     "/",
	})
}
