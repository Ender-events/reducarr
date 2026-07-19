package ui

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

type ProgressLogger struct {
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
	_ = os.Stdout.Sync()
}

func (p *ProgressLogger) LogPermanent(msg string) {
	// Clear the current line, print the permanent message with a newline
	fmt.Printf("\r\033[K%s\n", msg)
	_ = os.Stdout.Sync()
}

func (p *ProgressLogger) Done() {
	fmt.Println()
}

type Spinner struct {
	done chan bool
	msg  string
}

func NewSpinner(msg string) *Spinner {
	return &Spinner{
		done: make(chan bool),
		msg:  msg,
	}
}

func (s *Spinner) Start() {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	go func() {
		i := 0
		for {
			select {
			case <-s.done:
				return
			default:
				fmt.Printf("\r\033[K\033[36m%s\033[0m %s", frames[i], s.msg)
				_ = os.Stdout.Sync()
				i = (i + 1) % len(frames)
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

func (s *Spinner) Stop() {
	s.done <- true
	fmt.Print("\r\033[K") // Clear the spinner line
	_ = os.Stdout.Sync()
}
