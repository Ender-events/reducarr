package ui

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

type ProgressLogger struct {
	lastLineLen int
}

func NewProgressLogger() *ProgressLogger {
	return &ProgressLogger{}
}

// UpdateTruncate writes a message to the current line, truncating it if it exceeds terminal width.
func (p *ProgressLogger) UpdateTruncate(msg string) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && width > 0 {
		// Ensure we don't exceed terminal width to keep \r\033[K effective
		if len(msg) > width-1 {
			msg = msg[:width-4] + "..."
		}
	}

	// \r returns to start of line, \033[K clears to end of current row
	fmt.Printf("\r\033[K%s", msg)
	os.Stdout.Sync()
}

func (p *ProgressLogger) LogPermanent(msg string) {
	// Clear the current line, print the permanent message with a newline
	fmt.Printf("\r\033[K%s\n", msg)
	os.Stdout.Sync()
}

func (p *ProgressLogger) Done() {
	fmt.Println()
}
