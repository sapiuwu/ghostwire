package port

import "ghostwire/internal/domain/screen"

type CaptureOptions struct {
	MaxWidth int
}

type ScreenCapturerPort interface {
	Start(opts CaptureOptions) error
	Bounds() screen.ScreenSize
	Grab() screen.ScreenFrame
	Close() error
}
