package output

import (
	"errors"
	"testing"
)

func TestWithSpinner_QuietMode(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	called := false
	err := WithSpinner("loading...", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("fn should be called even in quiet mode")
	}
}

func TestWithSpinner_PropagatesError(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	expectedErr := errors.New("something failed")
	err := WithSpinner("working...", func() error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestWithSpinner_NonTTY(t *testing.T) {
	// In test environments, stdout is not a TTY, so the spinner is suppressed.
	// This tests the non-TTY code path.
	defer SetFormat(FormatText)
	SetFormat(FormatText)

	called := false
	err := WithSpinner("processing...", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("fn should be called in non-TTY mode")
	}
}

func TestIsTerminal_InTestEnvironment(t *testing.T) {
	// In a test environment, stdout is typically not a TTY.
	if isTerminal() {
		t.Skip("skipping: test stdout is a terminal (unexpected in CI)")
	}
}

func TestSuccess_Text_DoesNotPanic(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatText)

	// color.Green uses color.Output (cached at init, not os.Stdout),
	// so captureStdout can't intercept it. Just verify no panic.
	Success("it worked: %d", 42)
}

func TestPrint_QuietWithQuietString(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	out := captureStdout(func() {
		Print(&quietData{id: "proj-123"}, func() {
			t.Error("humanFn should not be called in quiet mode")
		})
	})
	if out == "" {
		t.Error("Print in quiet mode should output QuietString value")
	}
	if out != "proj-123\n" {
		t.Errorf("expected 'proj-123\\n', got %q", out)
	}
}

func TestPrint_QuietWithoutQuietString(t *testing.T) {
	defer SetFormat(FormatText)
	SetFormat(FormatQuiet)

	out := captureStdout(func() {
		Print(map[string]string{"key": "val"}, func() {
			t.Error("humanFn should not be called in quiet mode")
		})
	})
	if out != "" {
		t.Errorf("Print in quiet mode without QuietString should be silent, got %q", out)
	}
}

// quietData is a test type that implements QuietString().
type quietData struct {
	id string
}

func (q *quietData) QuietString() string {
	return q.id
}
