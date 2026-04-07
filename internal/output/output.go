package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Format controls how output is rendered.
type Format int

const (
	FormatText  Format = iota // Human-readable
	FormatJSON                // Machine-readable JSON
	FormatQuiet               // Minimal output (IDs, URLs only)
)

var currentFormat = FormatText

// SetFormat sets the global output format.
func SetFormat(f Format) { currentFormat = f }

// GetFormat returns the current output format.
func GetFormat() Format { return currentFormat }

// JSON prints data as indented JSON.
func JSON(data interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to encode JSON: %v\n", err)
	}
}

// Success prints a success message in green.
func Success(msg string, args ...interface{}) {
	if currentFormat == FormatQuiet {
		return
	}
	color.Green("✓ "+msg, args...)
}

// Warn prints a warning message in yellow.
func Warn(msg string, args ...interface{}) {
	if currentFormat == FormatQuiet {
		return
	}
	color.Yellow("⚠ "+msg, args...)
}

// Error prints an error message to stderr.
func Error(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+msg+"\n", args...)
}

// Info prints an informational message.
func Info(msg string, args ...interface{}) {
	if currentFormat == FormatQuiet {
		return
	}
	fmt.Printf(msg+"\n", args...)
}

// Quiet always prints the given value, used for essential output in all modes.
func Quiet(val string) {
	fmt.Println(val)
}

// Print renders data based on the current format.
// humanFn is called for text format; data is used for JSON format.
func Print(data interface{}, humanFn func()) {
	switch currentFormat {
	case FormatJSON:
		JSON(data)
	case FormatQuiet:
		if q, ok := data.(interface{ QuietString() string }); ok {
			Quiet(q.QuietString())
		}
	default:
		humanFn()
	}
}
