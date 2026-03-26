package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	fn()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestSetGetFormat(t *testing.T) {
	defer SetFormat(FormatText) // restore

	SetFormat(FormatJSON)
	if GetFormat() != FormatJSON {
		t.Errorf("expected FormatJSON, got %d", GetFormat())
	}

	SetFormat(FormatQuiet)
	if GetFormat() != FormatQuiet {
		t.Errorf("expected FormatQuiet, got %d", GetFormat())
	}

	SetFormat(FormatText)
	if GetFormat() != FormatText {
		t.Errorf("expected FormatText, got %d", GetFormat())
	}
}

func TestJSON(t *testing.T) {
	out := captureStdout(func() {
		JSON(map[string]string{"key": "value"})
	})

	var parsed map[string]string
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON output not valid JSON: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("expected key=value, got %v", parsed)
	}
}

func TestInfo_Text(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatText)

	out := captureStdout(func() {
		Info("hello %s", "world")
	})
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world', got %q", out)
	}
}

func TestInfo_Quiet(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	out := captureStdout(func() {
		Info("should not appear")
	})
	if out != "" {
		t.Errorf("Info should be suppressed in quiet mode, got %q", out)
	}
}

func TestSuccess_Quiet(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	out := captureStdout(func() {
		Success("should not appear")
	})
	if out != "" {
		t.Errorf("Success should be suppressed in quiet mode, got %q", out)
	}
}

func TestError_PrintsToStderr(t *testing.T) {
	out := captureStderr(func() {
		Error("test error %d", 42)
	})
	if !strings.Contains(out, "Error: test error 42") {
		t.Errorf("expected 'Error: test error 42', got %q", out)
	}
}

func TestQuiet_AlwaysPrints(t *testing.T) {
	defer SetFormat(FormatText)

	for _, f := range []Format{FormatText, FormatJSON, FormatQuiet} {
		SetFormat(f)
		out := captureStdout(func() {
			Quiet("essential-value")
		})
		if !strings.Contains(out, "essential-value") {
			t.Errorf("Quiet should always print, format=%d, got %q", f, out)
		}
	}
}

func TestPrint_JSON(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatJSON)

	out := captureStdout(func() {
		Print(map[string]int{"count": 5}, func() {
			t.Error("humanFn should not be called in JSON mode")
		})
	})

	var parsed map[string]int
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("Print JSON output not valid JSON: %v", err)
	}
	if parsed["count"] != 5 {
		t.Errorf("expected count=5, got %v", parsed)
	}
}

func TestPrint_Text(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatText)

	called := false
	captureStdout(func() {
		Print(nil, func() {
			called = true
		})
	})
	if !called {
		t.Error("humanFn should be called in text mode")
	}
}
