package tarobase

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	bin "github.com/gagliardetto/binary"
)

// The Anchor program's `set_documents` method discriminator. Comes from the
// IDL the server returns; stable per deployed program and matches:
//
//	sha256("global:set_documents")[0..8]  ==  [79,46,72,73,24,79,66,245]
//
// We hardcode rather than hashing at runtime because the value is part of
// the on-chain ABI and any mismatch silently sends to a different method.
var setDocumentsDiscriminator = [8]byte{79, 46, 72, 73, 24, 79, 66, 245}

// The Tarobase on-chain program id. Also stable per deployment.
const tarobaseProgramID = "poof4b5pk1L9tmThvBmaABjcyjfhFGbMbQP5BXk2QZp"

// EncodeSetDocumentsArgs borsh-encodes the Anchor `set_documents` args in
// the canonical order (app_id, documents, delete_paths, tx_data, simulate)
// and prepends the discriminator. The returned bytes are the instruction
// `data` field ready to hand to solana-go. args is taken by pointer because
// AnchorSetDocuments is big enough (96B) that passing by value trips
// golangci's gocritic/hugeParam check.
func EncodeSetDocumentsArgs(args *AnchorSetDocuments) ([]byte, error) {
	if args == nil {
		return nil, fmt.Errorf("args is nil")
	}
	// gagliardetto/binary's NewBorshEncoder handles most of this, but
	// FieldValue is a complex enum we have to drive manually (the library's
	// complex-enum encoder requires the enum as a struct-with-tag, which
	// our AnchorFieldValue already is — but we still need to ensure the
	// Option<FieldValue> wrapper is right, because the default encoder
	// treats pointer fields as always-present).
	buf := new(bytes.Buffer)
	if _, err := buf.Write(setDocumentsDiscriminator[:]); err != nil {
		return nil, err
	}
	if err := writeBorshString(buf, args.AppID); err != nil {
		return nil, fmt.Errorf("encode app_id: %w", err)
	}
	if err := writeBorshU32(buf, uint32(len(args.Documents))); err != nil {
		return nil, err
	}
	for i, d := range args.Documents {
		if err := encodeSetDocumentData(buf, d); err != nil {
			return nil, fmt.Errorf("document %d: %w", i, err)
		}
	}
	if err := writeBorshU32(buf, uint32(len(args.DeletePaths))); err != nil {
		return nil, err
	}
	for _, p := range args.DeletePaths {
		if err := writeBorshString(buf, p); err != nil {
			return nil, err
		}
	}
	if err := writeBorshU32(buf, uint32(len(args.TxData))); err != nil {
		return nil, err
	}
	for i, td := range args.TxData {
		if err := encodeTxData(buf, td); err != nil {
			return nil, fmt.Errorf("tx_data %d: %w", i, err)
		}
	}
	if err := writeBorshBool(buf, args.Simulate); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeSetDocumentData(buf *bytes.Buffer, d AnchorSetDocumentData) error {
	if err := writeBorshString(buf, d.Path); err != nil {
		return err
	}
	if err := writeBorshU32(buf, uint32(len(d.Operations))); err != nil {
		return err
	}
	for i, op := range d.Operations {
		if err := encodeFieldOperation(buf, op); err != nil {
			return fmt.Errorf("operation %d: %w", i, err)
		}
	}
	return nil
}

func encodeFieldOperation(buf *bytes.Buffer, op AnchorFieldOperation) error {
	if err := writeBorshString(buf, op.Key); err != nil {
		return err
	}
	// Option<FieldValue>: 1 byte tag then the value.
	if op.Value == nil {
		if err := buf.WriteByte(0); err != nil {
			return err
		}
	} else {
		if err := buf.WriteByte(1); err != nil {
			return err
		}
		if err := encodeFieldValue(buf, *op.Value); err != nil {
			return fmt.Errorf("field value: %w", err)
		}
	}
	return buf.WriteByte(op.Operation)
}

func encodeFieldValue(buf *bytes.Buffer, v AnchorFieldValue) error {
	// Complex-enum form: 1-byte variant index, then the variant payload.
	if err := buf.WriteByte(byte(v.Variant)); err != nil {
		return err
	}
	switch v.Variant {
	case 0: // U64Val
		if v.U64Val == nil {
			return errors.New("U64Val nil")
		}
		return writeBorshU64(buf, v.U64Val.Value)
	case 1: // I64Val
		if v.I64Val == nil {
			return errors.New("I64Val nil")
		}
		return writeBorshI64(buf, v.I64Val.Value)
	case 2: // BoolVal
		if v.BoolVal == nil {
			return errors.New("BoolVal nil")
		}
		return writeBorshBool(buf, v.BoolVal.Value)
	case 3: // StringVal
		if v.StringVal == nil {
			return errors.New("StringVal nil")
		}
		return writeBorshString(buf, v.StringVal.Value)
	case 4: // AddressVal
		if v.AddressVal == nil {
			return errors.New("AddressVal nil")
		}
		_, err := buf.Write(v.AddressVal.Value.Bytes())
		return err
	default:
		return fmt.Errorf("unknown FieldValue variant %d", v.Variant)
	}
}

func encodeTxData(buf *bytes.Buffer, td AnchorTxData) error {
	if err := writeBorshString(buf, td.PluginFunctionKey); err != nil {
		return err
	}
	if err := writeBorshU32(buf, uint32(len(td.TxData))); err != nil {
		return err
	}
	if _, err := buf.Write(td.TxData); err != nil {
		return err
	}
	if err := writeBorshU32(buf, uint32(len(td.RaIndices))); err != nil {
		return err
	}
	for _, n := range td.RaIndices {
		if err := writeBorshU64(buf, n); err != nil {
			return err
		}
	}
	return nil
}

// Borsh primitive helpers — all little-endian.
func writeBorshString(buf *bytes.Buffer, s string) error {
	if err := writeBorshU32(buf, uint32(len(s))); err != nil {
		return err
	}
	_, err := buf.WriteString(s)
	return err
}

func writeBorshU32(buf *bytes.Buffer, v uint32) error {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	_, err := buf.Write(b[:])
	return err
}

func writeBorshU64(buf *bytes.Buffer, v uint64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	_, err := buf.Write(b[:])
	return err
}

func writeBorshI64(buf *bytes.Buffer, v int64) error {
	return writeBorshU64(buf, uint64(v))
}

func writeBorshBool(buf *bytes.Buffer, v bool) error {
	if v {
		return buf.WriteByte(1)
	}
	return buf.WriteByte(0)
}

// Sanity-check the bin package compiles us in — we import it elsewhere but
// the borsh primitives here don't actually use its runtime. Keep the stub
// reference so a future reviewer sees the dependency is intentional.
var _ = bin.LE
