package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui/web"
	"github.com/spf13/cobra"
)

var port int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the web dashboard",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		// 1. Try to load ANY user from DB first
		user, pass, _ := database.GetFirstUser()

		if user != "" && pass != "" {
			fmt.Printf("\033[34m[AUTH]\033[0m Using persistent credentials from database.\n")
		} else {
			// 2. If DB is empty, use ENV or Defaults
			user = os.Getenv("REDUCARR_UI_USER")
			if user == "" {
				user = "admin"
			}

			pass = os.Getenv("REDUCARR_UI_PASS")
			if pass == "" {
				generated, err := web.GenerateRandomPassword(8)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error generating password: %v\n", err)
					os.Exit(1)
				}
				pass = generated
				fmt.Printf("\033[33m[WEB] No password set. Generating a temporary one for user '%s': %s\033[0m\n", user, pass)
			}

			// 3. Save to DB for next time
			_ = database.UpsertUser(user, pass)
			fmt.Printf("\033[32m✔\033[0m Web credentials for '%s' saved to database.\n", user)
		}

		client := getClient()
		handler := web.NewRouter(database, client, user, pass)

		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("\033[32m✔\033[0m Dashboard starting on http://localhost%s\n", addr)
		
		if err := http.ListenAndServe(addr, handler); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	serveCmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to listen on")
	rootCmd.AddCommand(serveCmd)
}
