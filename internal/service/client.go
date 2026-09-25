package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ghostwire/internal/domain"
	"ghostwire/internal/domain/input"
	"ghostwire/internal/domain/protocol"
	"ghostwire/internal/domain/screen"
	"ghostwire/internal/port"
	"ghostwire/internal/util"
)

type ClientDeps struct {
	Transport  port.TransportPort
	Compressor port.CompressorPort
	Logger     port.Logger
}

const eventQueueLimit = 4

type ClientService struct {
	deps ClientDeps
}

func NewClientService(deps ClientDeps) *ClientService {
	return &ClientService{deps: deps}
}

func (c *ClientService) Connect(ctx context.Context, opts port.ConnectOptions) (port.SessionPort, error) {
	conn, err := c.deps.Transport.Dial(opts.Address, opts.Tls)
	if err != nil {
		return nil, err
	}

	decoder := &protocol.MessageDecoder{}
	inbox := make([]protocol.DecodedMessage, 0, 16)
	queue := util.NewAsyncQueue[port.SessionEvent]()

	var mu sync.Mutex
	notify := func() {}
	closed := false
	var closeError error

	wake := func() {
		mu.Lock()
		fn := notify
		notify = func() {}
		mu.Unlock()
		fn()
	}

	finish := func(err error) {
		mu.Lock()
		if closed {
			mu.Unlock()
			return
		}
		closed = true
		if err != nil {
			closeError = domain.ToGhostwireError(err, domain.ErrProtocol)
		}
		queue.End(err)
		conn.Close()
		mu.Unlock()
		wake()
	}

	conn.OnData(func(chunk []byte) {
		mu.Lock()
		if closed {
			mu.Unlock()
			return
		}
		mu.Unlock()

		messages, err := decoder.Feed(chunk)
		if err != nil {
			finish(err)
			return
		}
		mu.Lock()
		inbox = append(inbox, messages...)
		mu.Unlock()
		wake()
	})

	conn.OnError(func(err error) {
		finish(err)
	})

	conn.OnClose(func() {
		mu.Lock()
		err := closeError
		mu.Unlock()
		if err == nil {
			finish(fmt.Errorf("connection closed by peer"))
		} else {
			finish(err)
		}
	})

	conn.OnDrain(func() {})

	// Send Hello
	conn.Write(protocol.EncodeJSONMessage(protocol.MsgHello, protocol.HelloPayload{
		ProtocolVersion: protocol.ProtocolVersion,
		ClientName:      opts.ClientName,
		Token:           opts.Token,
	}))

	// Handshake
	handshakeDeadline := time.Now().Add(time.Duration(opts.HandshakeTimeoutMs) * time.Millisecond)
	info := port.SessionInfo{Screen: screen.ScreenSize{}}
	stats := &port.SessionStats{}

	waitForInbox := func() error {
		mu.Lock()
		if len(inbox) > 0 {
			mu.Unlock()
			return nil
		}
		remaining := time.Until(handshakeDeadline)
		if remaining <= 0 {
			mu.Unlock()
			return domain.NewError(domain.ErrTimeout, "timed out waiting for server response")
		}
		done := make(chan struct{})
		mu.Lock()
		notify = func() {
			close(done)
		}
		mu.Unlock()

		select {
		case <-done:
		case <-time.After(remaining):
			mu.Lock()
			notify = func() {}
			mu.Unlock()
		}

		mu.Lock()
		if len(inbox) == 0 {
			if closed {
				mu.Unlock()
				if closeError != nil {
					return closeError
				}
				return domain.NewError(domain.ErrClosed, "connection closed during handshake")
			}
			mu.Unlock()
			return domain.NewError(domain.ErrTimeout, "timed out waiting for server response")
		}
		msg := inbox[0]
		inbox = inbox[1:]
		mu.Unlock()

		switch msg.Kind {
		case protocol.MsgWelcome:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			welcome, err := protocol.DecodeWelcome(payload)
			if err != nil {
				return err
			}
			if !welcome.Ok {
				reason := welcome.Reason
				if reason == "" {
					reason = "rejected by server"
				}
				conn.Write(protocol.EncodeJSONMessage(protocol.MsgBye, protocol.ByePayload{Reason: reason}))
				code := domain.ErrProtocol
				if reason == "unauthorized" {
					code = domain.ErrAuth
				}
				finish(domain.NewError(code, reason))
				return domain.NewError(code, reason)
			}
			if welcome.Screen != nil {
				info.Screen = screen.ScreenSize{Width: welcome.Screen.Width, Height: welcome.Screen.Height}
			}
			if welcome.Fps != nil {
				info.Fps = *welcome.Fps
			}
			info.ServerVersion = welcome.ServerVersion
			return nil

		case protocol.MsgError:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			ep, err := protocol.DecodeError(payload)
			if err != nil {
				return err
			}
			finish(domain.NewError(domain.ErrProtocol, fmt.Sprintf("%s: %s", ep.Code, ep.Message)))
			return domain.NewError(domain.ErrProtocol, fmt.Sprintf("%s: %s", ep.Code, ep.Message))

		case protocol.MsgBye:
			finish(domain.NewError(domain.ErrClosed, "session rejected"))
			return domain.NewError(domain.ErrClosed, "session rejected")

		default:
			return nil
		}
	}

	// Process handshake messages
	for {
		if err := waitForInbox(); err != nil {
			return nil, err
		}
		if info.ServerVersion != "" {
			break
		}
	}

	// Frame times for FPS calculation
	frameTimes := make([]int64, 0, 60)
	var lastSeenAt = time.Now()

	enqueue := func(event port.SessionEvent) {
		if event.Frame == nil {
			queue.Push(event)
			return
		}
		if queue.Size() >= eventQueueLimit {
			queue.RemoveFirst(func(ev port.SessionEvent) bool {
				return ev.Frame != nil
			})
			stats.FramesDropped++
		}
		queue.Push(event)
	}

	processMessage := func(msg protocol.DecodedMessage) error {
		lastSeenAt = time.Now()
		switch msg.Kind {
		case protocol.MsgFrame:
			meta, err := protocol.DecodeFrameMeta(msg.Payload, 0)
			if err != nil {
				return err
			}
			body := msg.Payload[protocol.FrameMetaSize:]
			var pixels []byte
			if meta.Encoding == uint8(protocol.FrameDeflate) {
				if c.deps.Compressor.Name() != "deflate" {
					return domain.NewError(domain.ErrProtocol, "frame is deflated but no decompressor configured")
				}
				pixels, err = c.deps.Compressor.Decompress(body, int(meta.RawLength))
				if err != nil {
					return err
				}
			} else {
				pixels = body
			}
			if len(pixels) != int(meta.RawLength) {
				return domain.NewError(domain.ErrProtocol, "decompressed frame has unexpected size")
			}
			frame := screen.ScreenFrame{
				Width:       int(meta.Width),
				Height:      int(meta.Height),
				Format:      "bgra8",
				TimestampMs: meta.TimestampMs,
				Data:        pixels,
			}
			stats.FramesReceived++
			stats.BytesReceived += len(msg.Payload)
			frameTimes = append(frameTimes, time.Now().UnixMilli())
			enqueue(port.SessionEvent{Frame: &frame})

		case protocol.MsgScreenInfo:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			si, err := protocol.DecodeScreenInfo(payload)
			if err != nil {
				return err
			}
			info.Screen = screen.ScreenSize{Width: si.Width, Height: si.Height}
			enqueue(port.SessionEvent{Screen: &info.Screen})

		case protocol.MsgPing:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			ping, err := protocol.DecodePing(payload)
			if err != nil {
				return err
			}
			conn.Write(protocol.EncodeJSONMessage(protocol.MsgPong, protocol.PingPayload{T: ping.T}))

		case protocol.MsgPong:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			pong, err := protocol.DecodePing(payload)
			if err != nil {
				return err
			}
			rtt := time.Now().UnixMilli() - pong.T
			if rtt >= 0 && rtt < 60000 {
				stats.RttMs = rtt
			}

		case protocol.MsgError:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return err
			}
			ep, err := protocol.DecodeError(payload)
			if err != nil {
				return err
			}
			return domain.NewError(domain.ErrProtocol, fmt.Sprintf("%s: %s", ep.Code, ep.Message))

		case protocol.MsgBye:
			payload, err := protocol.DecodeControlPayload(msg)
			if err != nil {
				return fmt.Errorf("bye")
			}
			bye, err := protocol.DecodeBye(payload)
			if err != nil {
				return fmt.Errorf("bye")
			}
			return domain.NewError(domain.ErrClosed, fmt.Sprintf("server closed session: %s", bye.Reason))
		}
		return nil
	}

	// Message pump goroutine
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for {
			mu.Lock()
			if closed {
				mu.Unlock()
				return
			}
			if len(inbox) == 0 {
				done := make(chan struct{})
				notify = func() { close(done) }
				mu.Unlock()
				select {
				case <-done:
				case <-ctx.Done():
					return
				}
				continue
			}
			msg := inbox[0]
			inbox = inbox[1:]
			mu.Unlock()

			if err := processMessage(msg); err != nil {
				finish(err)
				return
			}
		}
	}()

	// Keepalive
	keepalive := time.NewTicker(time.Duration(opts.PingIntervalMs) * time.Millisecond)
	go func() {
		for range keepalive.C {
			mu.Lock()
			isClosed := closed
			last := lastSeenAt
			mu.Unlock()
			if isClosed {
				return
			}
			if time.Since(last) > time.Duration(opts.PingTimeoutMs)*time.Millisecond {
				finish(domain.NewError(domain.ErrTimeout, "server stopped responding"))
				return
			}
			conn.Write(protocol.EncodeJSONMessage(protocol.MsgPing, protocol.PingPayload{T: time.Now().UnixMilli()}))
		}
	}()

	session := &clientSession{
		info:    &info,
		stats:   stats,
		queue:   queue,
		conn:    conn,
		closed:  &closed,
		mu:      &mu,
		finish:  finish,
		closeErr: &closeError,
	}
	return session, nil
}

type clientSession struct {
	info     *port.SessionInfo
	stats    *port.SessionStats
	queue    *util.AsyncQueue[port.SessionEvent]
	conn     port.Conn
	closed   *bool
	mu       *sync.Mutex
	finish   func(error)
	closeErr *error
}

func (s *clientSession) Info() port.SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return port.SessionInfo{
		Screen:        s.info.Screen,
		Fps:           s.info.Fps,
		ServerVersion: s.info.ServerVersion,
	}
}

func (s *clientSession) Events() <-chan port.SessionEvent {
	ch := make(chan port.SessionEvent, 16)
	go func() {
		defer close(ch)
		for {
			item, ok := s.queue.Next()
			if !ok {
				return
			}
			ch <- item
		}
	}()
	return ch
}

func (s *clientSession) SendInput(event input.InputEvent) {
	s.mu.Lock()
	closed := *s.closed
	s.mu.Unlock()
	if closed {
		return
	}
	s.conn.Write(protocol.EncodeMessage(protocol.MsgInput, input.EncodeInputEvent(event), 0))
}

func (s *clientSession) Stats() port.SessionStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return port.SessionStats{
		FramesReceived: s.stats.FramesReceived,
		FramesDropped:  s.stats.FramesDropped,
		BytesReceived:  s.stats.BytesReceived,
		RttMs:          s.stats.RttMs,
		Fps:            s.stats.Fps,
	}
}

func (s *clientSession) Close(reason string) error {
	s.mu.Lock()
	closed := *s.closed
	s.mu.Unlock()
	if closed {
		return nil
	}
	func() {
		defer func() { recover() }()
		s.conn.Write(protocol.EncodeJSONMessage(protocol.MsgBye, protocol.ByePayload{Reason: reason}))
	}()
	s.finish(nil)
	return nil
}
