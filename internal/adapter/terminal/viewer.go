package terminal

import (
	"context"
	"fmt"
	"os"
	"time"

	"ghostwire/internal/domain"
	"ghostwire/internal/domain/input"
	"ghostwire/internal/port"
	"golang.org/x/term"
)

const (
	statusIntervalMs = 500
	escFlushMs       = 50
)

type TerminalViewerDeps struct {
	Viewer port.ViewerPort
	Logger port.Logger
}

type TerminalViewer struct {
	deps TerminalViewerDeps
}

func NewTerminalViewer(deps TerminalViewerDeps) *TerminalViewer {
	return &TerminalViewer{deps: deps}
}

func (v *TerminalViewer) Run(ctx context.Context, opts port.ConnectOptions) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return domain.NewError(domain.ErrUnsupported, "connect: terminal viewer requires an interactive TTY")
	}

	session, err := v.deps.Viewer.Connect(ctx, opts)
	if err != nil {
		return err
	}

	info := session.Info()
	screenSize := info.Screen
	quitRequested := false
	var lastStatusAt int64

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return domain.WrapError(err, domain.ErrUnsupported, "failed to enter raw mode")
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	cols, rows, _ := term.GetSize(int(os.Stdout.Fd()))
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}

	renderer := NewAnsiRenderer(RendererIo{
		Write: func(chunk string) {
			fmt.Fprint(os.Stdout, chunk)
		},
	}, cols, rows, true)

	mapCell := func(cellX, cellY int) (int, int) {
		return MapCellToScreen(cellX, cellY, screenSize.Width, screenSize.Height, cols, renderer.ViewportRows())
	}

	applyInput := func(events []input.InputEvent) {
		for _, event := range events {
			if quitRequested {
				return
			}
			if ke, ok := event.(input.KeyEvent); ok {
				if ke.Key == input.KeyQ && ke.Modifiers&input.ModCtrl != 0 && ke.Down {
					quitRequested = true
					go session.Close("viewer quit")
					continue
				}
			}
			session.SendInput(event)
		}
	}

	renderer.Begin()
	defer renderer.End()

	// Read stdin in goroutine
	go func() {
		buf := make([]byte, 4096)
		var pending []byte
		var flushTimer *time.Timer
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				var merged []byte
				if len(pending) == 0 {
					merged = chunk
				} else {
					merged = append(pending, chunk...)
				}
				result := ParseTerminalInput(merged, TerminalParseOptions{MapCell: mapCell}, false)
				applyInput(result.Events)
				pending = merged[result.Consumed:]
				if flushTimer != nil {
					flushTimer.Stop()
				}
				if len(pending) > 0 && !quitRequested {
					flushTimer = time.AfterFunc(escFlushMs*time.Millisecond, func() {
						if len(pending) == 0 || quitRequested {
							return
						}
						flushed := ParseTerminalInput(pending, TerminalParseOptions{MapCell: mapCell}, true)
						applyInput(flushed.Events)
						if flushed.Consumed > 0 {
							pending = pending[flushed.Consumed:]
						} else {
							pending = nil
						}
					})
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Resize handler
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
				newCols, newRows, _ := term.GetSize(int(os.Stdout.Fd()))
				if newCols > 0 && newRows > 0 {
					renderer.Resize(newCols, newRows)
				}
			}
		}
	}()

	// Main event loop
	for event := range session.Events() {
		if event.Frame != nil {
			renderer.Render(*event.Frame)
			now := time.Now().UnixMilli()
			if now-lastStatusAt >= statusIntervalMs {
				lastStatusAt = now
				renderer.Status(statusLine(session))
			}
		} else if event.Screen != nil {
			screenSize = *event.Screen
		}
	}

	return nil
}

func statusLine(session port.SessionPort) string {
	info := session.Info()
	stats := session.Stats()
	size := fmt.Sprintf("%dx%d", info.Screen.Width, info.Screen.Height)
	fps := stats.Fps
	rtt := "-"
	if stats.RttMs > 0 {
		rtt = fmt.Sprintf("%dms", stats.RttMs)
	}
	dropped := ""
	if stats.FramesDropped > 0 {
		dropped = fmt.Sprintf(" drop %d", stats.FramesDropped)
	}
	return fmt.Sprintf(" ghostwire | %s | %d fps | rtt %s%s | ctrl+q quit", size, fps, rtt, dropped)
}
