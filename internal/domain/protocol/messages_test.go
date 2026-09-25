package protocol_test

import (
	"bytes"
	"testing"

	"ghostwire/internal/domain/protocol"
)

func TestRoundtripHello(t *testing.T) {
	fps := 30
	payload := protocol.EncodeHello(protocol.HelloPayload{
		ProtocolVersion: 1,
		ClientName:      "cli",
		Token:           "secret",
	})
	hello, err := protocol.DecodeHello(payload)
	if err != nil {
		t.Fatalf("DecodeHello failed: %v", err)
	}
	if hello.ProtocolVersion != 1 {
		t.Errorf("expected ProtocolVersion 1, got %d", hello.ProtocolVersion)
	}
	if hello.ClientName != "cli" {
		t.Errorf("expected ClientName 'cli', got %q", hello.ClientName)
	}
	if hello.Token != "secret" {
		t.Errorf("expected Token 'secret', got %q", hello.Token)
	}
	_ = fps
}

func TestRejectHelloInvalidTokenType(t *testing.T) {
	payload := []byte(`{"protocolVersion":1,"clientName":"x","token":5}`)
	_, err := protocol.DecodeHello(payload)
	if err == nil {
		t.Fatal("expected error for invalid token type")
	}
}

func TestRoundtripWelcome(t *testing.T) {
	fps := 30
	payload := protocol.EncodeWelcome(protocol.WelcomePayload{
		Ok:            true,
		ServerVersion: "0.1.0",
		Screen:        &protocol.ScreenSize{Width: 1920, Height: 1080},
		Fps:           &fps,
	})
	welcome, err := protocol.DecodeWelcome(payload)
	if err != nil {
		t.Fatalf("DecodeWelcome failed: %v", err)
	}
	if !welcome.Ok {
		t.Error("expected Ok=true")
	}
	if welcome.ServerVersion != "0.1.0" {
		t.Errorf("expected ServerVersion '0.1.0', got %q", welcome.ServerVersion)
	}
	if welcome.Screen == nil || welcome.Screen.Width != 1920 || welcome.Screen.Height != 1080 {
		t.Errorf("expected Screen 1920x1080, got %v", welcome.Screen)
	}
	if welcome.Fps == nil || *welcome.Fps != 30 {
		t.Errorf("expected Fps 30, got %v", welcome.Fps)
	}
}

func TestRoundtripRejectedWelcome(t *testing.T) {
	payload := protocol.EncodeWelcome(protocol.WelcomePayload{
		Ok:     false,
		Reason: "unauthorized",
	})
	welcome, err := protocol.DecodeWelcome(payload)
	if err != nil {
		t.Fatalf("DecodeWelcome failed: %v", err)
	}
	if welcome.Ok {
		t.Error("expected Ok=false")
	}
	if welcome.Reason != "unauthorized" {
		t.Errorf("expected Reason 'unauthorized', got %q", welcome.Reason)
	}
}

func TestRejectWelcomeBadScreen(t *testing.T) {
	payload := []byte(`{"ok":true,"screen":{"width":0,"height":1080}}`)
	_, err := protocol.DecodeWelcome(payload)
	if err == nil {
		t.Fatal("expected error for bad screen")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("screen dimensions")) {
		t.Errorf("expected 'screen dimensions' error, got: %v", err)
	}
}

func TestRoundtripPingErrorByeScreenInfo(t *testing.T) {
	ping, err := protocol.DecodePing(protocol.EncodePing(protocol.PingPayload{T: 1234567890}))
	if err != nil {
		t.Fatalf("DecodePing failed: %v", err)
	}
	if ping.T != 1234567890 {
		t.Errorf("expected T=1234567890, got %d", ping.T)
	}

	ep, err := protocol.DecodeError(protocol.EncodeError(protocol.ErrorPayload{Code: "busy", Message: "nope"}))
	if err != nil {
		t.Fatalf("DecodeError failed: %v", err)
	}
	if ep.Code != "busy" || ep.Message != "nope" {
		t.Errorf("expected busy/nope, got %s/%s", ep.Code, ep.Message)
	}

	bye, err := protocol.DecodeBye(protocol.EncodeBye(protocol.ByePayload{Reason: "done"}))
	if err != nil {
		t.Fatalf("DecodeBye failed: %v", err)
	}
	if bye.Reason != "done" {
		t.Errorf("expected Reason 'done', got %q", bye.Reason)
	}

	si, err := protocol.DecodeScreenInfo(protocol.EncodeScreenInfo(protocol.ScreenInfoPayload{Width: 800, Height: 600}))
	if err != nil {
		t.Fatalf("DecodeScreenInfo failed: %v", err)
	}
	if si.Width != 800 || si.Height != 600 {
		t.Errorf("expected 800x600, got %dx%d", si.Width, si.Height)
	}
}

func TestRejectNonObjectPayload(t *testing.T) {
	payload := []byte(`[1,2,3]`)
	_, err := protocol.DecodePing(payload)
	if err == nil {
		t.Fatal("expected error for non-object payload")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("JSON object")) {
		t.Errorf("expected 'JSON object' error, got: %v", err)
	}
}
