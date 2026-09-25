package protocol_test

import (
	"bytes"
	"testing"

	"ghostwire/internal/domain/protocol"
)

func TestEncodeDecodeMessage(t *testing.T) {
	payload := []byte("hello ghostwire")
	encoded := protocol.EncodeMessage(protocol.MsgPing, payload, 0)
	decoder := &protocol.MessageDecoder{}
	messages, err := decoder.Feed(encoded)
	if err != nil {
		t.Fatalf("Feed failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.Kind != protocol.MsgPing {
		t.Errorf("expected kind %d, got %d", protocol.MsgPing, msg.Kind)
	}
	if !bytes.Equal(msg.Payload, payload) {
		t.Errorf("payload mismatch: got %x", msg.Payload)
	}
}

func TestReassembleByteByByte(t *testing.T) {
	first := protocol.EncodeMessage(protocol.MsgHello, []byte("one"), 0)
	second := protocol.EncodeMessage(protocol.MsgBye, []byte("two"), 0)
	stream := append(first, second...)

	decoder := &protocol.MessageDecoder{}
	var kinds []protocol.MessageKind
	for _, b := range stream {
		msgs, err := decoder.Feed([]byte{b})
		if err != nil {
			t.Fatalf("Feed byte failed: %v", err)
		}
		for _, msg := range msgs {
			kinds = append(kinds, msg.Kind)
			s := string(msg.Payload)
			if s != "one" && s != "two" {
				t.Errorf("unexpected payload: %s", s)
			}
		}
	}
	if len(kinds) != 2 || kinds[0] != protocol.MsgHello || kinds[1] != protocol.MsgBye {
		t.Errorf("expected [Hello, Bye], got %v", kinds)
	}
}

func TestRejectBadMagic(t *testing.T) {
	decoder := &protocol.MessageDecoder{}
	_, err := decoder.Feed(make([]byte, 12))
	if err == nil {
		t.Fatal("expected error for bad magic")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("out of sync")) {
		t.Errorf("expected 'out of sync' error, got: %v", err)
	}
}

func TestRejectUnsupportedVersion(t *testing.T) {
	encoded := protocol.EncodeMessage(protocol.MsgPing, []byte{}, 0)
	encoded[4] = 99
	decoder := &protocol.MessageDecoder{}
	_, err := decoder.Feed(encoded)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("protocol version 99")) {
		t.Errorf("expected version error, got: %v", err)
	}
}

func TestCompressLargeControlPayload(t *testing.T) {
	reason := ""
	for i := 0; i < 4096; i++ {
		reason += "x"
	}
	encoded := protocol.EncodeJSONMessage(protocol.MsgBye, map[string]any{"reason": reason})
	decoder := &protocol.MessageDecoder{}
	messages, err := decoder.Feed(encoded)
	if err != nil {
		t.Fatalf("Feed failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	msg := messages[0]
	if msg.Flags&uint16(protocol.FlagDeflate) == 0 {
		t.Error("expected Deflate flag to be set")
	}
	payload, err := protocol.DecodeControlPayload(*msg)
	if err != nil {
		t.Fatalf("DecodeControlPayload failed: %v", err)
	}
	bye, err := protocol.DecodeBye(payload)
	if err != nil {
		t.Fatalf("DecodeBye failed: %v", err)
	}
	if bye.Reason != reason {
		t.Errorf("reason mismatch: got %q", bye.Reason)
	}
}

func TestMultipleMessagesInOneChunk(t *testing.T) {
	first := protocol.EncodeMessage(protocol.MsgHello, []byte("aaa"), 0)
	second := protocol.EncodeMessage(protocol.MsgWelcome, []byte("bbb"), 0)
	third := protocol.EncodeMessage(protocol.MsgPing, []byte("ccc"), 0)
	stream := append(append(first, second...), third...)

	decoder := &protocol.MessageDecoder{}
	messages, err := decoder.Feed(stream)
	if err != nil {
		t.Fatalf("Feed failed: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Kind != protocol.MsgHello {
		t.Errorf("expected Hello, got %d", messages[0].Kind)
	}
	if messages[1].Kind != protocol.MsgWelcome {
		t.Errorf("expected Welcome, got %d", messages[1].Kind)
	}
	if messages[2].Kind != protocol.MsgPing {
		t.Errorf("expected Ping, got %d", messages[2].Kind)
	}
}

func TestDecoderReset(t *testing.T) {
	decoder := &protocol.MessageDecoder{}
	// Feed partial header
	decoder.Feed(make([]byte, 6))
	decoder.Reset()
	// Now feed valid message
	encoded := protocol.EncodeMessage(protocol.MsgPong, []byte("ok"), 0)
	messages, err := decoder.Feed(encoded)
	if err != nil {
		t.Fatalf("Feed failed after reset: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}
