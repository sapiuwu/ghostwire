package port

import (
	"context"

	"ghostwire/internal/domain/input"
	"ghostwire/internal/domain/screen"
)

type SessionEvent struct {
	Frame *screen.ScreenFrame
	Screen *screen.ScreenSize
}

type SessionInfo struct {
	Screen        screen.ScreenSize
	Fps           int
	ServerVersion string
}

type SessionStats struct {
	FramesReceived int
	FramesDropped  int
	BytesReceived  int
	RttMs          int64
	Fps            int
}

type SessionPort interface {
	Info() SessionInfo
	Events() <-chan SessionEvent
	SendInput(event input.InputEvent)
	Stats() SessionStats
	Close(reason string) error
}

type ViewerPort interface {
	Connect(ctx context.Context, opts ConnectOptions) (SessionPort, error)
}
