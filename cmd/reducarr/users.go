package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui/web"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage WebUI users",
}

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all WebUI users",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		users, err := database.GetAllUsers()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching users: %v\n", err)
			os.Exit(1)
		}

		if len(users) == 0 {
			fmt.Println("No users found.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "USERNAME\tUPDATED AT")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%s\n", u.Username, u.UpdatedAt)
		}
		w.Flush()
	},
}

var usersAddCmd = &cobra.Command{
	Use:   "add [username] [password]",
	Short: "Add or update a WebUI user",
	Args:  cobra.RangeArgs(0, 2),
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		username := ""
		if len(args) > 0 {
			username = args[0]
		} else {
			prompt := promptui.Prompt{
				Label: "Enter Username",
			}
			username, err = prompt.Run()
			if err != nil {
				return
			}
		}

		if username == "" {
			fmt.Println("Username cannot be empty.")
			return
		}

		password := ""
		if len(args) > 1 {
			password = args[1]
		} else {
			prompt := promptui.Prompt{
				Label: "Enter Password (leave empty for random)",
				Mask:  '*',
			}
			password, err = prompt.Run()
			if err != nil {
				return
			}

			if password == "" {
				generated, _ := web.GenerateRandomPassword(8)
				password = generated
				fmt.Printf("\033[33mNo password provided. Generated:\033[0m %s\n", password)
			}
		}

		if err := database.UpsertUser(username, password); err != nil {
			fmt.Printf("\033[31m✘\033[0m Error saving user: %v\n", err)
		} else {
			fmt.Printf("\033[32m✔\033[0m User '%s' saved successfully.\n", username)
		}
	},
}

var usersDeleteCmd = &cobra.Command{
	Use:   "delete [username]",
	Short: "Delete a WebUI user",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		username := args[0]
		if err := database.DeleteUser(username); err != nil {
			fmt.Printf("\033[31m✘\033[0m Error deleting user: %v\n", err)
		} else {
			fmt.Printf("\033[32m✔\033[0m User '%s' deleted successfully.\n", username)
		}
	},
}

func init() {
	usersCmd.AddCommand(usersListCmd)
	usersCmd.AddCommand(usersAddCmd)
	usersCmd.AddCommand(usersDeleteCmd)
	rootCmd.AddCommand(usersCmd)
}
