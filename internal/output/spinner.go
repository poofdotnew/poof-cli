package output

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/term"
)

// WithSpinner runs fn while showing a terminal spinner.
// In non-TTY or quiet mode, the spinner is suppressed.
func WithSpinner(msg string, fn func() error) error {
	if currentFormat == FormatQuiet || !isTerminal() {
		return fn()
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + msg
	s.Start()
	err := fn()
	s.Stop()
	return err
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
