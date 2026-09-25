//go:build !windows

package capture

import (
	"ghostwire/internal/domain"
	"ghostwire/internal/domain/screen"
	"ghostwire/internal/port"
)

type GdiCapturer struct{}

func NewGdiCapturer() *GdiCapturer {
	return &GdiCapturer{}
}

func (c *GdiCapturer) Start(_ port.CaptureOptions) error {
	return domain.NewError(domain.ErrUnsupported, "screen capture requires Windows (GDI)")
}

func (c *GdiCapturer) Bounds() screen.ScreenSize {
	return screen.ScreenSize{}
}

func (c *GdiCapturer) Grab() screen.ScreenFrame {
	panic(domain.NewError(domain.ErrUnsupported, "screen capture requires Windows"))
}

func (c *GdiCapturer) Close() error {
	return nil
}
