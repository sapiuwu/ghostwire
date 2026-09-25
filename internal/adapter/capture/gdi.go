package capture

import (
	"math"
	"time"

	"ghostwire/internal/domain"
	"ghostwire/internal/domain/screen"
	"ghostwire/internal/port"
)

type GdiCapturer struct {
	maxWidth   int
	started    bool
	screenW    int
	screenH    int
	frameW     int
	frameH     int
	buffers    [][]byte
	activeBuf  int
}

func NewGdiCapturer() *GdiCapturer {
	return &GdiCapturer{}
}

func (c *GdiCapturer) Start(opts port.CaptureOptions) error {
	c.maxWidth = opts.MaxWidth
	c.started = true
	return c.ensureTargets()
}

func (c *GdiCapturer) Bounds() screen.ScreenSize {
	return screen.ScreenSize{Width: c.screenW, Height: c.screenH}
}

func (c *GdiCapturer) Grab() screen.ScreenFrame {
	if !c.started {
		panic(domain.NewError(domain.ErrInternal, "capturer has not been started"))
	}
	c.ensureTargets()
	buf := c.buffers[c.activeBuf]
	c.activeBuf = 1 - c.activeBuf
	return screen.ScreenFrame{
		Width:       c.frameW,
		Height:      c.frameH,
		Format:      "bgra8",
		TimestampMs: uint64(time.Now().UnixMilli()),
		Data:        buf,
	}
}

func (c *GdiCapturer) Close() error {
	c.started = false
	c.buffers = nil
	return nil
}

func (c *GdiCapturer) ensureTargets() error {
	if c.screenW == 0 || c.screenH == 0 {
		c.screenW = 1920
		c.screenH = 1080
	}
	w, h := c.targetSize()
	if c.frameW == w && c.frameH == h {
		return nil
	}
	c.frameW = w
	c.frameH = h
	size := w * h * 4
	c.buffers = [][]byte{make([]byte, size), make([]byte, size)}
	c.activeBuf = 0
	return nil
}

func (c *GdiCapturer) targetSize() (int, int) {
	if c.maxWidth <= 0 || c.screenW <= c.maxWidth {
		return c.screenW, c.screenH
	}
	w := int(math.Max(1, float64(c.maxWidth)))
	h := int(math.Max(1, math.Round(float64(c.screenH)*float64(w)/float64(c.screenW))))
	return w, h
}
