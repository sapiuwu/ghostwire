package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"ghostwire/internal/domain/input"
	"ghostwire/internal/domain/protocol"
	"ghostwire/internal/domain/screen"
	"ghostwire/internal/port"
	"ghostwire/internal/service"
)

type mockTransport struct {
	mu       sync.Mutex
	listener *mockListener
}

func (m *mockTransport) Listen(address string, tls *port.TlsListenOptions) (port.Listener, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listener = &mockListener{address: address}
	return m.listener, nil
}

func (m *mockTransport) Dial(address string, tls *port.TlsDialOptions) (port.Conn, error) {
	return nil, nil
}

type mockListener struct {
	address string
	connCb  func(port.Conn)
	errorCb func(error)
}

func (l *mockListener) Address() string                     { return l.address }
func (l *mockListener) OnConnection(cb func(port.Conn))     { l.connCb = cb }
func (l *mockListener) OnError(cb func(error))              { l.errorCb = cb }
func (l *mockListener) Close() error                        { return nil }

type mockConn struct {
	id      string
	written [][]byte
	closed  bool
	mu      sync.Mutex
	dataCb  func([]byte)
	errorCb func(error)
	closeCb func()
	drainCb func()
}

func newMockConn(id string) *mockConn {
	return &mockConn{id: id, written: make([][]byte, 0)}
}

func (c *mockConn) ID() string            { return c.id }
func (c *mockConn) RemoteAddress() string { return "mock:0" }
func (c *mockConn) Write(chunk []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	cp := make([]byte, len(chunk))
	copy(cp, chunk)
	c.written = append(c.written, cp)
	return true
}
func (c *mockConn) OnData(cb func([]byte))  { c.mu.Lock(); c.dataCb = cb; c.mu.Unlock() }
func (c *mockConn) OnError(cb func(error))  { c.mu.Lock(); c.errorCb = cb; c.mu.Unlock() }
func (c *mockConn) OnClose(cb func())       { c.mu.Lock(); c.closeCb = cb; c.mu.Unlock() }
func (c *mockConn) OnDrain(cb func())       { c.mu.Lock(); c.drainCb = cb; c.mu.Unlock() }
func (c *mockConn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.closeCb != nil {
		go c.closeCb()
	}
}

func (c *mockConn) deliver(data []byte) {
	c.mu.Lock()
	cb := c.dataCb
	c.mu.Unlock()
	if cb != nil {
		cb(data)
	}
}

type mockCapturer struct {
	started bool
	closed  bool
	width   int
	height  int
	counter int
}

func (m *mockCapturer) Start(_ port.CaptureOptions) error {
	m.started = true
	m.width = 8
	m.height = 4
	return nil
}

func (m *mockCapturer) Bounds() screen.ScreenSize {
	return screen.ScreenSize{Width: m.width, Height: m.height}
}

func (m *mockCapturer) Grab() screen.ScreenFrame {
	m.counter++
	data := make([]byte, m.width*m.height*4)
	for i := range data {
		data[i] = byte((i + m.counter) & 0xff)
	}
	return screen.ScreenFrame{
		Width:       m.width,
		Height:      m.height,
		Format:      "bgra8",
		TimestampMs: uint64(time.Now().UnixMilli()),
		Data:        data,
	}
}

func (m *mockCapturer) Close() error {
	m.closed = true
	return nil
}

type mockInjector struct {
	events   []input.InputEvent
	released bool
	closed   bool
}

func (m *mockInjector) Inject(ev input.InputEvent) {
	m.events = append(m.events, ev)
}

func (m *mockInjector) ReleaseAll() {
	m.released = true
}

func (m *mockInjector) Close() {
	m.closed = true
}

type mockCompressor struct{}

func (c *mockCompressor) Name() string { return "none" }
func (c *mockCompressor) Compress(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}
func (c *mockCompressor) Decompress(data []byte, expectedLength int) ([]byte, error) {
	return data[:expectedLength], nil
}

type mockLogger struct{}

func (l *mockLogger) Level() port.LogLevel                     { return port.LogDebug }
func (l *mockLogger) Debug(_ string, _ ...map[string]any)      {}
func (l *mockLogger) Info(_ string, _ ...map[string]any)       {}
func (l *mockLogger) Warn(_ string, _ ...map[string]any)       {}
func (l *mockLogger) Error(_ string, _ ...map[string]any)      {}

func TestServerStats(t *testing.T) {
	svc := service.NewServerService(service.ServerDeps{
		Transport:  &mockTransport{},
		Capturer:   &mockCapturer{},
		Injector:   &mockInjector{},
		Compressor: &mockCompressor{},
		Logger:     &mockLogger{},
	})
	stats := svc.Stats()
	if stats.ConnectionsAccepted != 0 {
		t.Errorf("expected 0 connections, got %d", stats.ConnectionsAccepted)
	}
}

func TestServerServeRequiresToken(t *testing.T) {
	svc := service.NewServerService(service.ServerDeps{
		Transport:  &mockTransport{},
		Capturer:   &mockCapturer{},
		Injector:   &mockInjector{},
		Compressor: &mockCompressor{},
		Logger:     &mockLogger{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := svc.Serve(port.ServeOptions{
		Address:            ":0",
		Token:              "",
		Fps:                10,
		HandshakeTimeoutMs: 10000,
		PingIntervalMs:     5000,
		PingTimeoutMs:      15000,
	}, ctx)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestServerServeInvalidFps(t *testing.T) {
	svc := service.NewServerService(service.ServerDeps{
		Transport:  &mockTransport{},
		Capturer:   &mockCapturer{},
		Injector:   &mockInjector{},
		Compressor: &mockCompressor{},
		Logger:     &mockLogger{},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := svc.Serve(port.ServeOptions{
		Address:            ":0",
		Token:              "test",
		Fps:                100,
		HandshakeTimeoutMs: 10000,
		PingIntervalMs:     5000,
		PingTimeoutMs:      15000,
	}, ctx)
	if err == nil {
		t.Fatal("expected error for fps > 60")
	}
}

func TestDecodeHelloPayload(t *testing.T) {
	payload := protocol.EncodeHello(protocol.HelloPayload{
		ProtocolVersion: 1,
		ClientName:      "test",
		Token:           "secret",
	})
	hello, err := protocol.DecodeHello(payload)
	if err != nil {
		t.Fatalf("DecodeHello failed: %v", err)
	}
	if hello.ProtocolVersion != 1 {
		t.Errorf("expected version 1, got %d", hello.ProtocolVersion)
	}
}
