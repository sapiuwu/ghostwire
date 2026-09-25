package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"sync"
	"time"

	"ghostwire/internal/domain"
	"ghostwire/internal/domain/input"
	"ghostwire/internal/domain/protocol"
	"ghostwire/internal/domain/screen"
	"ghostwire/internal/port"
)

type ServerDeps struct {
	Transport port.TransportPort
	Capturer  port.ScreenCapturerPort
	Injector  port.InputInjectorPort
	Compressor port.CompressorPort
	Logger    port.Logger
}

type ServerService struct {
	deps    ServerDeps
	host    *hostState
	mu      sync.Mutex
	counters port.ServerStats
}

type hostState struct {
	opts          port.ServeOptions
	listener      port.Listener
	active        *activeSession
	closed        bool
	captureTimer  *time.Timer
}

type activeSession struct {
	conn            port.Conn
	decoder         protocol.MessageDecoder
	authenticated   bool
	closed          bool
	lastSeenAt      time.Time
	handshakeTimer  *time.Timer
	keepaliveTimer  *time.Ticker
	paused          bool
}

func NewServerService(deps ServerDeps) *ServerService {
	return &ServerService{deps: deps}
}

func (s *ServerService) Stats() port.ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.counters
}

func tokensEqual(a, b string) bool {
	hashA := sha256.Sum256([]byte(a))
	hashB := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(hashA[:], hashB[:]) == 1
}

func (s *ServerService) Serve(opts port.ServeOptions, ctx context.Context) error {
	s.mu.Lock()
	if s.host != nil {
		s.mu.Unlock()
		return domain.NewError(domain.ErrInternal, "server is already running")
	}
	if opts.Token == "" {
		s.mu.Unlock()
		return domain.NewError(domain.ErrAuth, "a token is required to start the server")
	}
	if opts.Fps < 1 || opts.Fps > 60 {
		s.mu.Unlock()
		return domain.NewError(domain.ErrInternal, "fps must be between 1 and 60")
	}
	s.mu.Unlock()

	if err := s.deps.Capturer.Start(port.CaptureOptions{MaxWidth: opts.MaxWidth}); err != nil {
		return err
	}
	defer s.deps.Capturer.Close()

	listener, err := s.deps.Transport.Listen(opts.Address, opts.Tls)
	if err != nil {
		return err
	}
	defer listener.Close()

	host := &hostState{opts: opts, listener: listener}
	s.mu.Lock()
	s.host = host
	s.mu.Unlock()

	screenBounds := s.deps.Capturer.Bounds()
	s.deps.Logger.Info("server listening",
		map[string]any{"address": listener.Address(),
			"screen": fmt.Sprintf("%dx%d", screenBounds.Width, screenBounds.Height),
			"fps": opts.Fps})

	var lastPixels []byte
	var lastBounds screen.ScreenSize = screenBounds
	var seq uint32

	listener.OnConnection(func(conn port.Conn) {
		s.acceptConnection(conn, host, &lastPixels, &lastBounds, &seq)
	})
	listener.OnError(func(err error) {
		s.deps.Logger.Error("listener error", map[string]any{"error": err.Error()})
	})

	ticker := time.NewTicker(time.Second / time.Duration(opts.Fps))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return s.shutdown(host)
		case <-ticker.C:
			s.captureTick(host, &lastPixels, &lastBounds, &seq)
		}
	}
}

func (s *ServerService) captureTick(host *hostState, lastPixels *[]byte, lastBounds *screen.ScreenSize, seq *uint32) {
	s.mu.Lock()
	active := host.active
	closed := host.closed
	s.mu.Unlock()
	if closed || active == nil || !active.authenticated || active.paused {
		return
	}

	frame := s.grabFrame()
	if frame == nil {
		return
	}

	bounds := s.deps.Capturer.Bounds()
	if bounds.Width != lastBounds.Width || bounds.Height != lastBounds.Height {
		*lastBounds = bounds
		*lastPixels = nil
		s.broadcastScreenInfo(active, bounds)
	}

	if *lastPixels != nil && screen.SamePixels(frame.Data, *lastPixels) {
		s.mu.Lock()
		s.counters.FramesSkipped++
		s.mu.Unlock()
		return
	}

	rawLength := len(frame.Data)
	encoding := protocol.FrameRaw
	body := frame.Data

	if s.deps.Compressor.Name() == "deflate" {
		compressed, err := s.deps.Compressor.Compress(frame.Data)
		if err != nil {
			s.deps.Logger.Warn("frame compression failed", map[string]any{"error": err.Error()})
			return
		}
		body = compressed
		encoding = protocol.FrameDeflate
	}

	metaBytes := protocol.EncodeFrameMeta(protocol.FrameMeta{
		Seq:         *seq,
		TimestampMs: frame.TimestampMs,
		Width:       uint16(frame.Width),
		Height:      uint16(frame.Height),
		PixelFormat: uint8(protocol.PixelBGRA8),
		Encoding:    uint8(encoding),
		RawLength:   uint32(rawLength),
	})
	*seq++

	payload := append(metaBytes, body...)
	flushed := active.conn.Write(protocol.EncodeMessage(protocol.MsgFrame, payload, 0))
	active.paused = !flushed
	*lastPixels = frame.Data

	s.mu.Lock()
	s.counters.FramesSent++
	s.counters.BytesSent += len(payload)
	s.mu.Unlock()
}

func (s *ServerService) grabFrame() *screen.ScreenFrame {
	defer func() {
		if r := recover(); r != nil {
			s.deps.Logger.Warn("capture failed", map[string]any{"error": fmt.Sprintf("%v", r)})
		}
	}()
	frame := s.deps.Capturer.Grab()
	return &frame
}

func (s *ServerService) broadcastScreenInfo(active *activeSession, size screen.ScreenSize) {
	data := protocol.EncodeJSONMessage(protocol.MsgScreenInfo, protocol.ScreenInfoPayload{
		Width: size.Width, Height: size.Height,
	})
	active.conn.Write(data)
}

func (s *ServerService) acceptConnection(conn port.Conn, host *hostState, lastPixels *[]byte, lastBounds *screen.ScreenSize, seq *uint32) {
	s.mu.Lock()
	s.counters.ConnectionsAccepted++
	if host.active != nil && !host.active.closed {
		conn.Write(protocol.EncodeMessage(protocol.MsgError,
			protocol.EncodeError(protocol.ErrorPayload{Code: "busy", Message: "another viewer is already connected"}), 0))
		conn.Close()
		s.deps.Logger.Warn("rejected viewer: session busy")
		s.mu.Unlock()
		return
	}

	active := &activeSession{
		conn:       conn,
		lastSeenAt: time.Now(),
	}
	host.active = active
	s.mu.Unlock()

	s.deps.Logger.Info("viewer connecting", map[string]any{"remote": conn.RemoteAddress()})

	active.handshakeTimer = time.AfterFunc(time.Duration(host.opts.HandshakeTimeoutMs)*time.Millisecond, func() {
		s.failSession(active, "timeout", "handshake timed out")
	})

	active.keepaliveTimer = time.NewTicker(time.Duration(host.opts.PingIntervalMs) * time.Millisecond)
	go func() {
		for range active.keepaliveTimer.C {
			s.mu.Lock()
			if active.closed {
				s.mu.Unlock()
				return
			}
			if time.Since(active.lastSeenAt) > time.Duration(host.opts.PingTimeoutMs)*time.Millisecond {
				s.mu.Unlock()
				s.closeSession(active, "ping timeout")
				return
			}
			s.mu.Unlock()
			conn.Write(protocol.EncodeJSONMessage(protocol.MsgPing, protocol.PingPayload{T: time.Now().UnixMilli()}))
		}
	}()

	conn.OnDrain(func() {
		s.mu.Lock()
		active.paused = false
		s.mu.Unlock()
	})

	conn.OnData(func(chunk []byte) {
		s.mu.Lock()
		if active.closed {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		messages, err := active.decoder.Feed(chunk)
		if err != nil {
			gw := domain.ToGhostwireError(err, domain.ErrProtocol)
			s.failSession(active, string(gw.Code), gw.Message)
			return
		}
		for _, msg := range messages {
			s.mu.Lock()
			if active.closed {
				s.mu.Unlock()
				break
			}
			s.mu.Unlock()
			s.handleMessage(active, msg, host, lastPixels, seq)
		}
	})

	conn.OnError(func(err error) {
		s.deps.Logger.Debug("connection error", map[string]any{"error": err.Error()})
		s.closeSession(active, err.Error())
	})

	conn.OnClose(func() {
		s.closeSession(active, "peer closed")
	})
}

func (s *ServerService) handleMessage(active *activeSession, msg protocol.DecodedMessage, host *hostState, lastPixels *[]byte, seq *uint32) {
	s.mu.Lock()
	active.lastSeenAt = time.Now()
	s.mu.Unlock()

	if !active.authenticated {
		if msg.Kind != protocol.MsgHello {
			s.failSession(active, "protocol", "expected handshake message")
			return
		}
		s.handleHello(active, msg, host, lastPixels, seq)
		return
	}

	switch msg.Kind {
	case protocol.MsgInput:
		payload, err := protocol.DecodeControlPayload(msg)
		if err != nil {
			return
		}
		event, err := input.DecodeInputEvent(payload)
		if err != nil {
			s.deps.Logger.Warn("invalid input event", map[string]any{"error": err.Error()})
			return
		}
		s.deps.Injector.Inject(event)
		s.mu.Lock()
		s.counters.InputEvents++
		s.mu.Unlock()

	case protocol.MsgPing:
		payload, err := protocol.DecodeControlPayload(msg)
		if err != nil {
			return
		}
		ping, err := protocol.DecodePing(payload)
		if err != nil {
			return
		}
		active.conn.Write(protocol.EncodeJSONMessage(protocol.MsgPong, protocol.PingPayload{T: ping.T}))

	case protocol.MsgPong:
		// ignore

	case protocol.MsgBye:
		payload, err := protocol.DecodeControlPayload(msg)
		if err != nil {
			return
		}
		bye, err := protocol.DecodeBye(payload)
		if err != nil {
			return
		}
		s.closeSession(active, bye.Reason)

	case protocol.MsgHello:
		s.failSession(active, "protocol", "duplicate handshake")

	default:
		s.deps.Logger.Warn("unexpected message from viewer", map[string]any{"kind": msg.Kind})
	}
}

func (s *ServerService) handleHello(active *activeSession, msg protocol.DecodedMessage, host *hostState, lastPixels *[]byte, seq *uint32) {
	payload, err := protocol.DecodeControlPayload(msg)
	if err != nil {
		s.failSession(active, "protocol", err.Error())
		return
	}
	hello, err := protocol.DecodeHello(payload)
	if err != nil {
		s.failSession(active, "protocol", err.Error())
		return
	}

	if hello.ProtocolVersion != protocol.ProtocolVersion {
		active.conn.Write(protocol.EncodeMessage(protocol.MsgWelcome,
			protocol.EncodeWelcome(protocol.WelcomePayload{
				Ok:     false,
				Reason: fmt.Sprintf("protocol version %d required", protocol.ProtocolVersion),
			}), 0))
		s.closeSession(active, "protocol version mismatch")
		return
	}

	if !tokensEqual(hello.Token, host.opts.Token) {
		s.deps.Logger.Warn("rejected unauthorized viewer", map[string]any{"clientName": hello.ClientName})
		active.conn.Write(protocol.EncodeMessage(protocol.MsgWelcome,
			protocol.EncodeWelcome(protocol.WelcomePayload{Ok: false, Reason: "unauthorized"}), 0))
		s.closeSession(active, "unauthorized viewer")
		return
	}

	if active.handshakeTimer != nil {
		active.handshakeTimer.Stop()
		active.handshakeTimer = nil
	}

	active.authenticated = true
	s.mu.Lock()
	s.counters.SessionsServed++
	s.mu.Unlock()

	bounds := s.deps.Capturer.Bounds()
	*lastPixels = nil

	fps := host.opts.Fps
	active.conn.Write(protocol.EncodeMessage(protocol.MsgWelcome,
		protocol.EncodeWelcome(protocol.WelcomePayload{
			Ok:            true,
			ServerVersion: protocol.ServerVersion,
			Screen:        &protocol.ScreenSize{Width: bounds.Width, Height: bounds.Height},
			Fps:           &fps,
		}), 0))

	s.deps.Logger.Info("viewer connected", map[string]any{"clientName": hello.ClientName})
}

func (s *ServerService) failSession(active *activeSession, code string, message string) {
	s.mu.Lock()
	if active.closed {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	func() {
		defer func() { recover() }()
		active.conn.Write(protocol.EncodeMessage(protocol.MsgError,
			protocol.EncodeError(protocol.ErrorPayload{Code: code, Message: message}), 0))
	}()
	s.closeSession(active, message)
}

func (s *ServerService) closeSession(active *activeSession, reason string) {
	s.mu.Lock()
	if active.closed {
		s.mu.Unlock()
		return
	}
	active.closed = true
	if active.handshakeTimer != nil {
		active.handshakeTimer.Stop()
	}
	if active.keepaliveTimer != nil {
		active.keepaliveTimer.Stop()
	}
	s.mu.Unlock()

	s.deps.Injector.ReleaseAll()
	active.conn.Close()
	s.deps.Logger.Info("session closed", map[string]any{"reason": reason})
}

func (s *ServerService) shutdown(host *hostState) error {
	s.mu.Lock()
	host.closed = true
	if host.captureTimer != nil {
		host.captureTimer.Stop()
	}
	active := host.active
	s.mu.Unlock()

	if active != nil && !active.closed {
		func() {
			defer func() { recover() }()
			active.conn.Write(protocol.EncodeJSONMessage(protocol.MsgBye,
				protocol.ByePayload{Reason: "server shutting down"}))
		}()
		s.closeSession(active, "server shutting down")
	}

	host.listener.Close()
	s.deps.Injector.Close()
	s.deps.Logger.Info("server stopped")
	return nil
}
