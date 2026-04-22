package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/poofdotnew/poof-cli/internal/tarobase"
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

func TestParseChainFlag(t *testing.T) {
	cases := []struct {
		in   string
		want tarobase.Chain
		err  bool
	}{
		{"offchain", tarobase.ChainOffchain, false},
		{"mainnet", tarobase.ChainMainnet, false},
		{"solana_mainnet", tarobase.ChainMainnet, false},
		{"", "", true},
		{"devnet", "", true},
		{"MAINNET", "", true}, // case-sensitive by design
	}
	for _, c := range cases {
		got, err := parseChainFlag(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseChainFlag(%q): want err, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseChainFlag(%q): unexpected err %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseChainFlag(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}

// resolveDataTarget's happy paths hit network (Resolve) or config, but the
// flag-combination errors are pure — verify the appid/env and chain/no-appid
// conflict rules fire before any outbound call.
func TestResolveDataTarget_FlagConflicts(t *testing.T) {
	t.Cleanup(func() {
		flagDataAppID = ""
		flagDataChain = ""
		flagDataEnv = ""
	})

	flagDataAppID = "app123"
	flagDataChain = "offchain"
	flagDataEnv = "draft"
	if _, err := resolveDataTarget(context.Background()); err == nil || !strings.Contains(err.Error(), "--environment conflicts with --app-id") {
		t.Errorf("expected env-vs-appid conflict, got %v", err)
	}

	flagDataAppID = ""
	flagDataChain = "mainnet"
	flagDataEnv = ""
	if _, err := resolveDataTarget(context.Background()); err == nil || !strings.Contains(err.Error(), "--chain only applies with --app-id") {
		t.Errorf("expected chain-without-appid error, got %v", err)
	}

	flagDataAppID = "app123"
	flagDataChain = ""
	flagDataEnv = ""
	if _, err := resolveDataTarget(context.Background()); err == nil || !strings.Contains(err.Error(), "--chain is required") {
		t.Errorf("expected chain-required error when app-id set, got %v", err)
	}
}

func TestResolveDataTarget_SharedAppIDSuccess(t *testing.T) {
	t.Cleanup(func() {
		flagDataAppID = ""
		flagDataChain = ""
		flagDataEnv = ""
	})
	flagDataAppID = "shared-123"
	flagDataChain = "mainnet"
	flagDataEnv = ""
	resolved, err := resolveDataTarget(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resolved.AppID != "shared-123" {
		t.Errorf("appid: got %q", resolved.AppID)
	}
	if resolved.Chain != tarobase.ChainMainnet {
		t.Errorf("chain: got %q", resolved.Chain)
	}
	if resolved.APIURL == "" || resolved.AuthURL == "" {
		t.Errorf("expected default API/Auth URLs populated, got %q / %q", resolved.APIURL, resolved.AuthURL)
	}
}
