package transport_test

import (
	"testing"

	"ghostwire/internal/adapter/transport"
)

func TestParseAddressValid(t *testing.T) {
	host, port, err := transport.ParseAddress("127.0.0.1:5901")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %q", host)
	}
	if port != 5901 {
		t.Errorf("expected port 5901, got %d", port)
	}
}

func TestParseAddressIPv6(t *testing.T) {
	host, port, err := transport.ParseAddress("[::1]:5901")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "::1" {
		t.Errorf("expected host ::1, got %q", host)
	}
	if port != 5901 {
		t.Errorf("expected port 5901, got %d", port)
	}
}

func TestParseAddressNoColon(t *testing.T) {
	_, _, err := transport.ParseAddress("localhost")
	if err == nil {
		t.Fatal("expected error for missing colon")
	}
}

func TestParseAddressBadPort(t *testing.T) {
	_, _, err := transport.ParseAddress("host:abc")
	if err == nil {
		t.Fatal("expected error for bad port")
	}
}

func TestParseAddressPortRange(t *testing.T) {
	_, _, err := transport.ParseAddress("host:70000")
	if err == nil {
		t.Fatal("expected error for port out of range")
	}
}

func TestParseAddressEmptyHost(t *testing.T) {
	host, port, err := transport.ParseAddress(":5901")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host != "" {
		t.Errorf("expected empty host, got %q", host)
	}
	if port != 5901 {
		t.Errorf("expected port 5901, got %d", port)
	}
}
