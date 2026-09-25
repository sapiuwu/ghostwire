package protocol_test

import (
	"bytes"
	"testing"

	"ghostwire/internal/domain/protocol"
)

func TestRoundtripFrameMeta(t *testing.T) {
	meta := protocol.FrameMeta{
		Seq:         42,
		TimestampMs: 1700000000123,
		X:           0,
		Y:           0,
		Width:       1920,
		Height:      1080,
		PixelFormat: uint8(protocol.PixelBGRA8),
		Encoding:    uint8(protocol.FrameDeflate),
		RawLength:   1920 * 1080 * 4,
	}
	encoded := protocol.EncodeFrameMeta(meta)
	decoded, err := protocol.DecodeFrameMeta(encoded, 0)
	if err != nil {
		t.Fatalf("DecodeFrameMeta failed: %v", err)
	}
	if decoded.Seq != meta.Seq {
		t.Errorf("Seq: got %d, want %d", decoded.Seq, meta.Seq)
	}
	if decoded.TimestampMs != meta.TimestampMs {
		t.Errorf("TimestampMs: got %d, want %d", decoded.TimestampMs, meta.TimestampMs)
	}
	if decoded.Width != meta.Width || decoded.Height != meta.Height {
		t.Errorf("Dimensions: got %dx%d, want %dx%d", decoded.Width, decoded.Height, meta.Width, meta.Height)
	}
	if decoded.RawLength != meta.RawLength {
		t.Errorf("RawLength: got %d, want %d", decoded.RawLength, meta.RawLength)
	}
}

func TestRejectUnsupportedPixelFormat(t *testing.T) {
	encoded := protocol.EncodeFrameMeta(protocol.FrameMeta{
		Seq:         1,
		TimestampMs: 1,
		X:           0,
		Y:           0,
		Width:       2,
		Height:      2,
		PixelFormat: 7,
		Encoding:    uint8(protocol.FrameRaw),
		RawLength:   16,
	})
	_, err := protocol.DecodeFrameMeta(encoded, 0)
	if err == nil {
		t.Fatal("expected error for unsupported pixel format")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("pixel format")) {
		t.Errorf("expected 'pixel format' error, got: %v", err)
	}
}

func TestRejectRawLengthMismatch(t *testing.T) {
	encoded := protocol.EncodeFrameMeta(protocol.FrameMeta{
		Seq:         1,
		TimestampMs: 1,
		X:           0,
		Y:           0,
		Width:       2,
		Height:      2,
		PixelFormat: uint8(protocol.PixelBGRA8),
		Encoding:    uint8(protocol.FrameRaw),
		RawLength:   15,
	})
	_, err := protocol.DecodeFrameMeta(encoded, 0)
	if err == nil {
		t.Fatal("expected error for raw length mismatch")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("raw length")) {
		t.Errorf("expected 'raw length' error, got: %v", err)
	}
}

func TestRejectTruncatedHeader(t *testing.T) {
	_, err := protocol.DecodeFrameMeta(make([]byte, 10), 0)
	if err == nil {
		t.Fatal("expected error for truncated header")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("truncated")) {
		t.Errorf("expected 'truncated' error, got: %v", err)
	}
}

func TestRejectInvalidFrameDimensions(t *testing.T) {
	encoded := protocol.EncodeFrameMeta(protocol.FrameMeta{
		Seq:         1,
		TimestampMs: 1,
		Width:       0,
		Height:      0,
		PixelFormat: uint8(protocol.PixelBGRA8),
		Encoding:    uint8(protocol.FrameRaw),
		RawLength:   0,
	})
	_, err := protocol.DecodeFrameMeta(encoded, 0)
	if err == nil {
		t.Fatal("expected error for invalid frame dimensions")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("invalid frame dimensions")) {
		t.Errorf("expected 'invalid frame dimensions' error, got: %v", err)
	}
}

func TestFrameMetaOffset(t *testing.T) {
	meta := protocol.FrameMeta{
		Seq:         1,
		TimestampMs: 100,
		Width:       10,
		Height:      10,
		PixelFormat: uint8(protocol.PixelBGRA8),
		Encoding:    uint8(protocol.FrameRaw),
		RawLength:   10 * 10 * 4,
	}
	encoded := protocol.EncodeFrameMeta(meta)
	prefix := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	data := append(prefix, encoded...)
	decoded, err := protocol.DecodeFrameMeta(data, 4)
	if err != nil {
		t.Fatalf("DecodeFrameMeta with offset failed: %v", err)
	}
	if decoded.Width != 10 || decoded.Height != 10 {
		t.Errorf("expected 10x10, got %dx%d", decoded.Width, decoded.Height)
	}
}
