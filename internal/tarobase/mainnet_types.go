package tarobase

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	bin "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
)

// Types in this file mirror the shape `api.tarobase.com/items` returns when
// the target app is configured for mainnet (preview or production). Every
// field is what the Tarobase service produces; everything below is a thin
// Go-native reflection of the Anchor `set_documents` args plus the auxiliary
// signing metadata the SDK's `handleSolanaTransaction` consumes.

// MainnetBuildResponse is the `PUT /items` body shape on mainnet.
type MainnetBuildResponse struct {
	Transactions []MainnetTransaction `json:"transactions"`
}

// MainnetTransaction is one of the one-or-more structured Solana txs the
// server returns. For the common single-tx setMany we pick transactions[0].
type MainnetTransaction struct {
	RemainingAccounts      []AccountMetaInput `json:"remainingAccounts"`
	LutAddress             string             `json:"lutAddress,omitempty"`
	AdditionalLutAddresses []string           `json:"additionalLutAddresses,omitempty"`
	TransactionArgs        TransactionArgs    `json:"transactionArgs"`
	IDL                    json.RawMessage    `json:"idl,omitempty"`
	PreInstructions        []PreInstruction   `json:"preInstructions,omitempty"`
	TxData                 []TxDataEntry      `json:"txData,omitempty"`
	Network                string             `json:"network"`
	// SignedTransaction is a base64 of a server co-signed transaction. When
	// present, the server has pre-assembled CPI tx bytes and we only need to
	// countersign as the user. Not handled in v1 — returns an error.
	SignedTransaction string `json:"signedTransaction,omitempty"`
}

// AccountMetaInput mirrors the RemainingAccounts JSON entries.
type AccountMetaInput struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

// TransactionArgs is the transactionArgs object the server passes — these
// become the Anchor `documents` + `delete_paths` arguments.
type TransactionArgs struct {
	SetDocumentData []SetDocumentDataInput `json:"setDocumentData"`
	DeletePaths     []string               `json:"deletePaths"`
}

// SetDocumentDataInput is a single document-set in the build response. It
// borshes into the Anchor `SetDocumentData` struct.
type SetDocumentDataInput struct {
	Path       string                `json:"path"`
	Operations []FieldOperationInput `json:"operations"`
}

// FieldOperationInput is one key/value set within a document. `operation` is
// an integer op code (0 = set). `value` is a tagged-union FieldValue.
type FieldOperationInput struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"` // keep raw to parse the union explicitly
	Operation uint8           `json:"operation"`
}

// PreInstruction is a raw Solana instruction description the server prepends.
type PreInstruction struct {
	ProgramID string              `json:"programId"`
	Keys      []PreInstructionKey `json:"keys"`
	Data      []byte              `json:"data"` // JSON numeric array → byte slice via encoding/json
}

// PreInstructionKey matches solana-go AccountMeta fields.
type PreInstructionKey struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

// TxDataEntry is one entry in the server's `txData` array, passed to the
// Anchor `set_documents(.. , tx_data, ..)` call. Present when an onchain
// collection's hook does CPI into another program (e.g. Phoenix perps,
// Meteora pool creation) and the server has to encode the CPI arg bytes
// the Poof program will forward. See
// TaroBase/packages/tarobase-core/src/client/operations.ts handleSolanaTransaction
// for the reference handling.
type TxDataEntry struct {
	PluginFunctionKey string     `json:"pluginFunctionKey"`
	TxData            BufferJSON `json:"txData"`
	RaIndices         HexUInt64s `json:"raIndices"`
}

// BufferJSON matches the Node `Buffer.toJSON()` shape `{type:"Buffer",data:[...]}`
// that `Buffer.from(...)` on the TS side round-trips through. We also accept
// a bare number array (what web3.js sometimes emits) and a base64 string.
type BufferJSON []byte

func (b *BufferJSON) UnmarshalJSON(raw []byte) error {
	// Bare number array: [0, 1, 2, 3, ...]
	var arr []byte
	if err := json.Unmarshal(raw, &arr); err == nil {
		*b = arr
		return nil
	}
	// Node Buffer shape: {"type":"Buffer","data":[...]}
	var wrap struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Type == "Buffer" && len(wrap.Data) > 0 {
		var inner []byte
		if err := json.Unmarshal(wrap.Data, &inner); err != nil {
			return fmt.Errorf("Buffer.data: %w", err)
		}
		*b = inner
		return nil
	}
	// Base64 string (another common shape).
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		decoded, err := json.RawMessage(nil), error(nil)
		_ = decoded
		_ = err
		// Avoid pulling in encoding/base64 here — do it manually.
		return fmt.Errorf("base64 string txData not yet supported (got %q)", s)
	}
	return fmt.Errorf("txData: expected number array, Buffer-json, or base64 string; got %s", string(raw))
}

// HexUInt64s matches the server's raIndices shape — either a hex string
// (possibly "0x"-prefixed, possibly unprefixed) or a decimal number. The TS
// side round-trips through BN(v, "hex") when isHexString(v). We normalize
// to []uint64 for our borsh encoder.
type HexUInt64s []uint64

func (h *HexUInt64s) UnmarshalJSON(raw []byte) error {
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return fmt.Errorf("raIndices: %w", err)
	}
	out := make([]uint64, 0, len(arr))
	for _, item := range arr {
		// Try number first.
		var n json.Number
		if err := json.Unmarshal(item, &n); err == nil {
			if v, err := strconv.ParseUint(string(n), 10, 64); err == nil {
				out = append(out, v)
				continue
			}
		}
		// Try string (hex or decimal).
		var s string
		if err := json.Unmarshal(item, &s); err != nil {
			return fmt.Errorf("raIndices entry: expected number or string, got %s", string(item))
		}
		s = strings.TrimPrefix(s, "0x")
		s = strings.TrimPrefix(s, "0X")
		n64, ok := new(big.Int).SetString(s, 16)
		if !ok {
			// Fall back to decimal parse.
			if v, err := strconv.ParseUint(s, 10, 64); err == nil {
				out = append(out, v)
				continue
			}
			return fmt.Errorf("raIndices entry %q not a valid hex or decimal uint64", s)
		}
		if !n64.IsUint64() {
			return fmt.Errorf("raIndices entry %q overflows uint64", s)
		}
		out = append(out, n64.Uint64())
	}
	*h = out
	return nil
}

// ----------------------------------------------------------------------------
// Borsh-encodable Anchor arg structs.
//
// The Anchor `set_documents` instruction takes:
//
//   fn set_documents(
//     app_id:       String,
//     documents:    Vec<SetDocumentData>,
//     delete_paths: Vec<String>,
//     tx_data:      Vec<TxData>,
//     simulate:     bool,
//   )
//
// where SetDocumentData / FieldOperation / FieldValue match the IDL the
// server returns. Field order matters for borsh.
// ----------------------------------------------------------------------------

// AnchorSetDocuments holds the borsh-encodable form of the instruction args.
type AnchorSetDocuments struct {
	AppID       string
	Documents   []AnchorSetDocumentData
	DeletePaths []string
	TxData      []AnchorTxData
	Simulate    bool
}

// AnchorSetDocumentData is the borsh form of a single set.
type AnchorSetDocumentData struct {
	Path       string
	Operations []AnchorFieldOperation
}

// AnchorFieldOperation: the value is Option<FieldValue>, encoded as
// a one-byte tag (0 = None, 1 = Some) + the value. Operation is a u8 op code.
type AnchorFieldOperation struct {
	Key       string
	Value     *AnchorFieldValue // nil → encoded as None
	Operation uint8
}

// AnchorFieldValue is a borsh complex enum. Variant tag is the field index;
// see gagliardetto/binary's encodeComplexEnumBorsh. Field 0 is the BorshEnum
// tag; subsequent fields are variant payloads (only the one matching the tag
// is consumed).
type AnchorFieldValue struct {
	Variant    bin.BorshEnum // 0=U64Val, 1=I64Val, 2=BoolVal, 3=StringVal, 4=AddressVal
	U64Val     *AnchorU64Val
	I64Val     *AnchorI64Val
	BoolVal    *AnchorBoolVal
	StringVal  *AnchorStringVal
	AddressVal *AnchorAddressVal
}

type AnchorU64Val struct {
	Value uint64
}
type AnchorI64Val struct {
	Value int64
}
type AnchorBoolVal struct {
	Value bool
}
type AnchorStringVal struct {
	Value string
}
type AnchorAddressVal struct {
	Value solana.PublicKey
}

// AnchorTxData is the CPI-only data field; empty when TxData isn't used.
type AnchorTxData struct {
	PluginFunctionKey string
	TxData            []byte
	RaIndices         []uint64
}

// ----------------------------------------------------------------------------
// FieldValue union parser.
// ----------------------------------------------------------------------------

// parseFieldValue takes the server's tagged-union JSON (one of u64Val / i64Val
// / boolVal / stringVal / addressVal) and produces the borsh form. The server
// encodes u64 / i64 as hex strings under `.value`; strings and addresses are
// plain base58 / utf-8.
func parseFieldValue(raw json.RawMessage) (*AnchorFieldValue, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("field value must be a tagged object: %w", err)
	}
	if len(obj) != 1 {
		return nil, fmt.Errorf("field value should have exactly one tag, got %d", len(obj))
	}

	for tag, inner := range obj {
		var valueHolder struct {
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(inner, &valueHolder); err != nil {
			return nil, fmt.Errorf("parse %s.value: %w", tag, err)
		}
		switch tag {
		case "u64Val":
			n, err := parseU64Hex(valueHolder.Value)
			if err != nil {
				return nil, fmt.Errorf("u64Val: %w", err)
			}
			return &AnchorFieldValue{Variant: 0, U64Val: &AnchorU64Val{Value: n}}, nil
		case "i64Val":
			n, err := parseI64Hex(valueHolder.Value)
			if err != nil {
				return nil, fmt.Errorf("i64Val: %w", err)
			}
			return &AnchorFieldValue{Variant: 1, I64Val: &AnchorI64Val{Value: n}}, nil
		case "boolVal":
			var b bool
			if err := json.Unmarshal(valueHolder.Value, &b); err != nil {
				return nil, fmt.Errorf("boolVal: %w", err)
			}
			return &AnchorFieldValue{Variant: 2, BoolVal: &AnchorBoolVal{Value: b}}, nil
		case "stringVal":
			var s string
			if err := json.Unmarshal(valueHolder.Value, &s); err != nil {
				return nil, fmt.Errorf("stringVal: %w", err)
			}
			return &AnchorFieldValue{Variant: 3, StringVal: &AnchorStringVal{Value: s}}, nil
		case "addressVal":
			var s string
			if err := json.Unmarshal(valueHolder.Value, &s); err != nil {
				return nil, fmt.Errorf("addressVal: %w", err)
			}
			pk, err := solana.PublicKeyFromBase58(s)
			if err != nil {
				return nil, fmt.Errorf("addressVal not a valid pubkey: %w", err)
			}
			return &AnchorFieldValue{Variant: 4, AddressVal: &AnchorAddressVal{Value: pk}}, nil
		default:
			return nil, fmt.Errorf("unknown field value variant %q", tag)
		}
	}
	return nil, nil // unreachable
}

// parseU64Hex parses the server's u64 encoding. The JSON can be either a hex
// string (common: `"value":"01"` for u64(1)) or an already-decoded number.
func parseU64Hex(raw json.RawMessage) (uint64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		n, ok := new(big.Int).SetString(s, 16)
		if !ok {
			return 0, fmt.Errorf("not valid hex: %q", s)
		}
		if !n.IsUint64() {
			return 0, fmt.Errorf("u64 overflow: %s", s)
		}
		return n.Uint64(), nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		v, err := strconv.ParseUint(string(n), 10, 64)
		return v, err
	}
	return 0, fmt.Errorf("u64 value not decodable: %s", string(raw))
}

func parseI64Hex(raw json.RawMessage) (int64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		// Tarobase sends signed values as two's-complement hex, so a string
		// like "ffffffffffffffff" is -1 as i64, not an overflow. Parse the
		// bit pattern as u64 and reinterpret. Reject strings longer than
		// 16 hex digits — those genuinely don't fit.
		if len(s) > 16 {
			return 0, fmt.Errorf("i64 overflow (> 16 hex chars): %s", s)
		}
		n, ok := new(big.Int).SetString(s, 16)
		if !ok {
			return 0, fmt.Errorf("not valid hex: %q", s)
		}
		if !n.IsUint64() {
			return 0, fmt.Errorf("i64 hex not u64-representable: %s", s)
		}
		return int64(n.Uint64()), nil //nolint:gosec // deliberate bit-pattern reinterpret
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("i64 value not decodable: %s", string(raw))
}
