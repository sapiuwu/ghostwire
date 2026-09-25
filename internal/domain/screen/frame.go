package screen

import "math"

type ScreenSize struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ScreenFrame struct {
	Width       int
	Height      int
	Format      string
	TimestampMs uint64
	Data        []byte
}

func FrameByteLength(width, height int) int {
	return width * height * 4
}

func NewScreenFrame(width, height int, timestampMs uint64) ScreenFrame {
	return ScreenFrame{
		Width:       width,
		Height:      height,
		Format:      "bgra8",
		TimestampMs: timestampMs,
		Data:        make([]byte, FrameByteLength(width, height)),
	}
}

func ScaleFrameNearest(src ScreenFrame, dstWidth, dstHeight int) ScreenFrame {
	if dstWidth == src.Width && dstHeight == src.Height {
		return src
	}
	dst := NewScreenFrame(dstWidth, dstHeight, src.TimestampMs)
	for y := 0; y < dstHeight; y++ {
		sy := int(math.Min(float64(src.Height-1), math.Floor(float64(y*src.Height)/float64(dstHeight))))
		srcRow := sy * src.Width * 4
		dstRow := y * dstWidth * 4
		for x := 0; x < dstWidth; x++ {
			sx := int(math.Min(float64(src.Width-1), math.Floor(float64(x*src.Width)/float64(dstWidth))))
			si := srcRow + sx*4
			di := dstRow + x*4
			dst.Data[di] = src.Data[si]
			dst.Data[di+1] = src.Data[si+1]
			dst.Data[di+2] = src.Data[si+2]
			dst.Data[di+3] = src.Data[si+3]
		}
	}
	return dst
}

func SamePixels(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	chunk := 8192
	for i := 0; i < len(a); i += chunk {
		end := i + chunk
		if end > len(a) {
			end = len(a)
		}
		for j := i; j < end; j++ {
			if a[j] != b[j] {
				return false
			}
		}
	}
	return true
}
