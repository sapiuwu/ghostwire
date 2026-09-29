package service_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"ghostwire/internal/adapter/transport"
	"ghostwire/internal/domain"
	"ghostwire/internal/port"
	"ghostwire/internal/service"
)

const e2eToken = "e2e-token-secret"

func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot reserve free port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func startTestServer(t *testing.T) (string, func()) {
	t.Helper()
	addr := freeAddress(t)
	svc := service.NewServerService(service.ServerDeps{
		Transport:  &transport.TCPTransport{},
		Capturer:   &mockCapturer{},
		Injector:   &mockInjector{},
		Compressor: &mockCompressor{},
		Logger:     &mockLogger{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.Serve(port.ServeOptions{
			Address:            addr,
			Token:              e2eToken,
			Fps:                5,
			HandshakeTimeoutMs: 3000,
			PingIntervalMs:     5000,
			PingTimeoutMs:      15000,
		}, ctx)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		select {
		case serveErr := <-errCh:
			t.Fatalf("server exited early: %v", serveErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never listened on %s: %v", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop := func() {
		cancel()
		select {
		case <-errCh:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down after cancel")
		}
	}
	return addr, stop
}

func newViewer() *service.ClientService {
	return service.NewClientService(service.ClientDeps{
		Transport:  &transport.TCPTransport{},
		Compressor: &mockCompressor{},
		Logger:     &mockLogger{},
	})
}

func viewerOptions(addr, token string) port.ConnectOptions {
	return port.ConnectOptions{
		Address:            addr,
		Token:              token,
		ClientName:         "e2e-viewer",
		HandshakeTimeoutMs: 3000,
		PingIntervalMs:     5000,
		PingTimeoutMs:      15000,
	}
}

func connectViewer(addr, token string) (port.SessionPort, error) {
	viewer := newViewer()
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := viewer.Connect(context.Background(), viewerOptions(addr, token))
		if err == nil {
			return session, nil
		}
		gw := domain.ToGhostwireError(err, domain.ErrInternal)
		retryable := gw.Code == domain.ErrTransport || strings.Contains(gw.Message, "busy")
		if !retryable || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServeAcceptsViewerOverRealTcp(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	session, err := connectViewer(addr, e2eToken)
	if err != nil {
		t.Fatalf("connect with valid token failed: %v", err)
	}
	defer session.Close("test")

	info := session.Info()
	if info.ServerVersion == "" {
		t.Error("expected server version in session info")
	}
	if info.Screen.Width != 8 || info.Screen.Height != 4 {
		t.Errorf("expected mock screen 8x4, got %dx%d", info.Screen.Width, info.Screen.Height)
	}
}

func TestServeRejectsWrongTokenOverRealTcp(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	session, err := connectViewer(addr, e2eToken)
	if err != nil {
		t.Fatalf("sanity connect with valid token failed: %v", err)
	}
	session.Close("sanity")

	viewer := newViewer()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := viewer.Connect(context.Background(), viewerOptions(addr, "wrong-token"))
		if err == nil {
			t.Fatal("expected wrong token to be rejected")
		}
		gw := domain.ToGhostwireError(err, domain.ErrInternal)
		if gw.Code == domain.ErrAuth {
			if !strings.Contains(gw.Message, "unauthorized") {
				t.Errorf("expected unauthorized reason, got %q", gw.Message)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected auth error, got %s: %v", gw.Code, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServeRejectsSecondViewerOverRealTcp(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	first, err := connectViewer(addr, e2eToken)
	if err != nil {
		t.Fatalf("first viewer connect failed: %v", err)
	}
	defer first.Close("test")

	viewer := newViewer()
	_, err = viewer.Connect(context.Background(), viewerOptions(addr, e2eToken))
	if err == nil {
		t.Fatal("expected second viewer to be rejected")
	}
	gw := domain.ToGhostwireError(err, domain.ErrInternal)
	if !strings.Contains(gw.Message, "busy") {
		t.Errorf("expected busy error, got %s: %v", gw.Code, err)
	}
}
