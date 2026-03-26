package output

import (
	"strings"
	"testing"
)

func TestTable(t *testing.T) {
	out := captureStdout(func() {
		Table([]string{"Name", "Age"}, [][]string{
			{"Alice", "30"},
			{"Bob", "25"},
		})
	})

	if !strings.Contains(out, "Name") {
		t.Error("table missing header 'Name'")
	}
	if !strings.Contains(out, "Age") {
		t.Error("table missing header 'Age'")
	}
	if !strings.Contains(out, "Alice") {
		t.Error("table missing data 'Alice'")
	}
	if !strings.Contains(out, "Bob") {
		t.Error("table missing data 'Bob'")
	}
	// Should have separator dashes
	if !strings.Contains(out, "----") {
		t.Error("table missing separator")
	}
}

func TestTable_Empty(t *testing.T) {
	out := captureStdout(func() {
		Table([]string{"Col"}, [][]string{})
	})

	if !strings.Contains(out, "Col") {
		t.Error("table missing header even with no rows")
	}
}
