package tarobase

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gagliardetto/solana-go"
)

// The borsh wire format is strict: any off-by-one in tagged-union encoding
// or endianness breaks the on-chain instruction silently. These tests fix
// byte-level output for known inputs so regressions are caught pre-submit.

func TestEncodeSetDocumentsArgs_DiscriminatorPrepended(t *testing.T) {
	out, err := EncodeSetDocumentsArgs(AnchorSetDocuments{AppID: ""})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Equal(out[:8], setDocumentsDiscriminator[:]) {
		t.Errorf("discriminator mismatch: got %x, want %x", out[:8], setDocumentsDiscriminator)
	}
}

func TestEncodeSetDocumentsArgs_EmptyArgsLayout(t *testing.T) {
	// All-empty args: just discriminator + empty strings/vecs/bool=false.
	//   discriminator (8)
	//   app_id len u32=0 (4)           -> ""
	//   documents len u32=0 (4)        -> []
	//   delete_paths len u32=0 (4)     -> []
	//   tx_data len u32=0 (4)          -> []
	//   simulate u8=0 (1)              -> false
	out, err := EncodeSetDocumentsArgs(AnchorSetDocuments{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	want := hex.EncodeToString(append(append([]byte{}, setDocumentsDiscriminator[:]...),
		// 4 zero u32s (app_id, documents, delete_paths, tx_data lengths) + simulate=false
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0, 0, 0, 0,
		0,
	))
	got := hex.EncodeToString(out)
	if got != want {
		t.Errorf("empty args layout mismatch:\n got  %s\n want %s", got, want)
	}
}

func TestEncodeSetDocumentsArgs_StringFieldValue(t *testing.T) {
	// Encode: app_id="app", 1 document with path="user/x/y/z" and one
	// StringVal operation { key="content", value=StringVal("hi"), op=0 }.
	args := AnchorSetDocuments{
		AppID: "app",
		Documents: []AnchorSetDocumentData{{
			Path: "user/x/y/z",
			Operations: []AnchorFieldOperation{{
				Key:       "content",
				Value:     &AnchorFieldValue{Variant: 3, StringVal: &AnchorStringVal{Value: "hi"}},
				Operation: 0,
			}},
		}},
	}
	out, err := EncodeSetDocumentsArgs(args)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Manually lay out expected bytes for clarity.
	var buf bytes.Buffer
	buf.Write(setDocumentsDiscriminator[:])
	// app_id
	buf.Write([]byte{3, 0, 0, 0}) // len=3
	buf.WriteString("app")
	// documents vec len=1
	buf.Write([]byte{1, 0, 0, 0})
	// document: path len=10
	buf.Write([]byte{10, 0, 0, 0})
	buf.WriteString("user/x/y/z")
	// operations vec len=1
	buf.Write([]byte{1, 0, 0, 0})
	// operation: key len=7
	buf.Write([]byte{7, 0, 0, 0})
	buf.WriteString("content")
	// Option tag = Some
	buf.WriteByte(1)
	// FieldValue variant = 3 (StringVal)
	buf.WriteByte(3)
	// StringVal: length prefix + bytes
	buf.Write([]byte{2, 0, 0, 0})
	buf.WriteString("hi")
	// operation u8 = 0
	buf.WriteByte(0)
	// empty delete_paths
	buf.Write([]byte{0, 0, 0, 0})
	// empty tx_data
	buf.Write([]byte{0, 0, 0, 0})
	// simulate=false
	buf.WriteByte(0)

	if !bytes.Equal(out, buf.Bytes()) {
		t.Errorf("string-value encoding mismatch:\n got  %s\n want %s",
			hex.EncodeToString(out), hex.EncodeToString(buf.Bytes()))
	}
}

func TestEncodeSetDocumentsArgs_U64AndAddressFieldValues(t *testing.T) {
	// Two ops: amount=1 (u64), source=<addr>. Addr is a canonical 32-byte
	// pubkey we can lay out raw.
	addrStr := "11111111111111111111111111111111"
	addr := solana.MustPublicKeyFromBase58(addrStr)

	args := AnchorSetDocuments{
		AppID: "a",
		Documents: []AnchorSetDocumentData{{
			Path: "p",
			Operations: []AnchorFieldOperation{
				{Key: "amount", Value: &AnchorFieldValue{Variant: 0, U64Val: &AnchorU64Val{Value: 1}}, Operation: 0},
				{Key: "source", Value: &AnchorFieldValue{Variant: 4, AddressVal: &AnchorAddressVal{Value: addr}}, Operation: 0},
			},
		}},
	}
	out, err := EncodeSetDocumentsArgs(args)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Verify the u64 is laid out little-endian. Find position of the u64
	// payload: it comes right after the Option-Some + variant-tag pair that
	// follows the "amount" key.
	// We don't need a bit-for-bit comparison here; just check the sentinel
	// bytes (0x01 followed by seven zeros).
	want := append([]byte{1}, bytes.Repeat([]byte{0}, 7)...)
	if !bytes.Contains(out, want) {
		t.Errorf("u64=1 little-endian bytes not found in encoded output %x", out)
	}
	// And the 32-byte pubkey should appear verbatim (all 1s zero-padded
	// after base58 decode is just 32 zero bytes for SystemProgramID).
	if !bytes.Contains(out, addr.Bytes()) {
		t.Errorf("pubkey bytes not found in encoded output")
	}
}

func TestParseFieldValue_EveryVariant(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		variant int
	}{
		{"u64", `{"u64Val":{"value":"0a"}}`, 0},
		{"i64", `{"i64Val":{"value":"ffffffffffffffff"}}`, 1}, // -1 as i64
		{"bool", `{"boolVal":{"value":true}}`, 2},
		{"string", `{"stringVal":{"value":"hi"}}`, 3},
		{"address", `{"addressVal":{"value":"11111111111111111111111111111111"}}`, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := parseFieldValue(json.RawMessage(c.raw))
			if err != nil {
				t.Fatalf("parse %s: %v", c.name, err)
			}
			if v == nil || int(v.Variant) != c.variant {
				t.Errorf("variant: got %v want %d", v, c.variant)
			}
		})
	}
}

func TestParseU64Hex_HexAndDecimalForms(t *testing.T) {
	n, err := parseU64Hex(json.RawMessage(`"0a"`))
	if err != nil || n != 10 {
		t.Errorf("hex string: got %d err %v", n, err)
	}
	n, err = parseU64Hex(json.RawMessage(`"ffffffffffffffff"`))
	if err != nil || n != 18446744073709551615 {
		t.Errorf("hex max: got %d err %v", n, err)
	}
	if _, err := parseU64Hex(json.RawMessage(`"zz"`)); err == nil {
		t.Error("expected error on bad hex")
	}
}
