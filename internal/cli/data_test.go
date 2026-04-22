package cli

import (
	"strings"
	"testing"
)

func TestParseSetManyPayload_AcceptsBareArray(t *testing.T) {
	input := []byte(`[
		{"path":"a","document":{"x":1}},
		{"path":"b","document":{"y":2}}
	]`)
	docs, err := parseSetManyPayload(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("want 2 docs, got %d", len(docs))
	}
	if docs[0].Path != "a" || docs[1].Path != "b" {
		t.Errorf("paths mismatch: %+v", docs)
	}
}

func TestParseSetManyPayload_AcceptsDocumentsWrapper(t *testing.T) {
	input := []byte(`{"documents":[{"path":"a","document":{"x":1}}]}`)
	docs, err := parseSetManyPayload(input)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(docs) != 1 || docs[0].Path != "a" {
		t.Errorf("wrapper parse failed: %+v", docs)
	}
}

func TestParseSetManyPayload_RejectsEmpty(t *testing.T) {
	if _, err := parseSetManyPayload([]byte("  ")); err == nil {
		t.Error("expected error on empty payload")
	}
	if _, err := parseSetManyPayload([]byte("[]")); err == nil {
		t.Error("expected error on empty array")
	}
	if _, err := parseSetManyPayload([]byte(`{"documents":[]}`)); err == nil {
		t.Error("expected error on empty wrapper")
	}
}

func TestParseSetManyPayload_RejectsInvalidJSON(t *testing.T) {
	_, err := parseSetManyPayload([]byte("{not-json"))
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestParseSetManyPayload_RequiresPathOnEachEntry(t *testing.T) {
	input := []byte(`[{"document":{"x":1}}]`)
	_, err := parseSetManyPayload(input)
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Errorf("expected path-required error, got %v", err)
	}
}
