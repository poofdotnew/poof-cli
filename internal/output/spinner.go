package output

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"golang.org/x/term"
)

// heartbeatInterval is how often WithSpinner prints a progress line in non-TTY
// (agent) mode. TTY users get the animated spinner instead. Set low enough that
// long-running builds (5-15 min) don't look dead, but not so low that the log
// becomes noisy.
var heartbeatInterval = 20 * time.Second

// WithSpinner runs fn while reporting progress.
//
// Behavior by output mode:
//   - Quiet: no output, just run fn.
//   - TTY text: animated spinner on stderr.
//   - Non-TTY text (agents, pipes, logs): print the start message immediately,
//     then emit a heartbeat line every heartbeatInterval showing elapsed time.
//     This gives agents running `poof build` / `poof iterate` / `poof verify`
//     visible progress in their captured stdout instead of nothing for 5-15 min.
//   - JSON: also uses the non-TTY heartbeat so streaming consumers still see
//     activity — heartbeat lines go to stderr so stdout stays valid JSON.
func WithSpinner(msg string, fn func() error) error {
	if currentFormat == FormatQuiet {
		return fn()
	}

	if isTerminal() && currentFormat != FormatJSON {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " " + msg
		s.Start()
		err := fn()
		s.Stop()
		return err
	}

	// Non-TTY or JSON mode: emit start line + periodic heartbeats.
	// JSON mode writes to stderr to keep stdout clean for JSON consumers.
	out := os.Stdout
	if currentFormat == FormatJSON {
		out = os.Stderr
	}

	fmt.Fprintf(out, "… %s\n", msg)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	start := time.Now()
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				fmt.Fprintf(out, "… %s (elapsed %s)\n", msg, elapsed)
			}
		}
	}()

	err := fn()
	close(stop)
	wg.Wait()

	if err == nil {
		elapsed := time.Since(start).Round(time.Second)
		fmt.Fprintf(out, "… done (%s)\n", elapsed)
	}
	return err
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
