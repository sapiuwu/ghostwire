package terminal

import (
	"fmt"
	"strings"

	"ghostwire/internal/domain/screen"
)

type RendererIo struct {
	Write func(chunk string)
}

const halfBlock = "\u2580"

type AnsiRenderer struct {
	io          RendererIo
	cols        int
	rows        int
	renderRows  int
	curFg       []int32
	curBg       []int32
	nextFg      []int32
	nextBg      []int32
	sgrFg       int32
	sgrBg       int32
	statusEnabled bool
}

func NewAnsiRenderer(io RendererIo, cols, rows int, statusEnabled bool) *AnsiRenderer {
	r := &AnsiRenderer{
		io:            io,
		cols:          cols,
		rows:          rows,
		statusEnabled: statusEnabled,
		sgrFg:         -1,
		sgrBg:         -1,
	}
	r.renderRows = r.computeRenderRows()
	size := r.cols * r.renderRows
	r.curFg = make([]int32, size)
	r.curBg = make([]int32, size)
	r.nextFg = make([]int32, size)
	r.nextBg = make([]int32, size)
	for i := range r.curFg {
		r.curFg[i] = -1
		r.curBg[i] = -1
	}
	return r
}

func (r *AnsiRenderer) computeRenderRows() int {
	if r.statusEnabled && r.rows > 1 {
		return r.rows - 1
	}
	return r.rows
}

func (r *AnsiRenderer) ViewportRows() int {
	return r.renderRows
}

func (r *AnsiRenderer) Begin() {
	r.io.Write("\x1b[?25l\x1b[?7l" +
		"\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h" +
		"\x1b[2J\x1b[H")
	for i := range r.curFg {
		r.curFg[i] = -1
		r.curBg[i] = -1
	}
	r.sgrFg = -1
	r.sgrBg = -1
}

func (r *AnsiRenderer) Resize(cols, rows int) {
	if cols == r.cols && rows == r.rows {
		return
	}
	r.cols = cols
	r.rows = rows
	r.renderRows = r.computeRenderRows()
	size := r.cols * r.renderRows
	r.curFg = make([]int32, size)
	r.curBg = make([]int32, size)
	r.nextFg = make([]int32, size)
	r.nextBg = make([]int32, size)
	for i := range r.curFg {
		r.curFg[i] = -1
		r.curBg[i] = -1
	}
	r.io.Write("\x1b[2J\x1b[H")
}

func (r *AnsiRenderer) Render(frame screen.ScreenFrame) {
	cols := r.cols
	renderRows := r.renderRows
	source := frame.Data
	frameW := frame.Width
	frameH := frame.Height

	for row := 0; row < renderRows; row++ {
		y0 := row * frameH / renderRows
		y1 := (row + 1) * frameH / renderRows
		if y1 <= y0 {
			y1 = y0 + 1
		}
		mid := y0 + (y1-y0)/2
		if mid < y0+1 {
			mid = y0 + 1
		}
		if mid > y1 {
			mid = y1
		}

		for col := 0; col < cols; col++ {
			x0 := col * frameW / cols
			x1 := (col + 1) * frameW / cols
			if x1 <= x0 {
				x1 = x0 + 1
			}
			top := averageColor(source, x0, y0, x1, mid, frameW)
			bottom := top
			if mid < y1 {
				bottom = averageColor(source, x0, mid, x1, y1, frameW)
			}
			index := row*cols + col
			r.nextFg[index] = top
			r.nextBg[index] = bottom
		}
	}

	var out strings.Builder
	for row := 0; row < renderRows; row++ {
		rowBase := row * cols
		col := 0
		for col < cols {
			index := rowBase + col
			if r.nextFg[index] == r.curFg[index] && r.nextBg[index] == r.curBg[index] {
				col++
				continue
			}
			fmt.Fprintf(&out, "\x1b[%d;%dH", row+1, col+1)
			for col < cols {
				i := rowBase + col
				fg := r.nextFg[i]
				bg := r.nextBg[i]
				if fg == r.curFg[i] && bg == r.curBg[i] {
					break
				}
				out.WriteString(r.sgrStr(fg, bg))
				out.WriteString(halfBlock)
				r.curFg[i] = fg
				r.curBg[i] = bg
				col++
			}
		}
	}
	if out.Len() > 0 {
		r.io.Write(out.String())
	}
}

func (r *AnsiRenderer) Status(text string) {
	if !r.statusEnabled {
		return
	}
	row := r.rows
	r.io.Write(fmt.Sprintf("\x1b[%d;1H\x1b[2K\x1b[38;2;170;187;204m%s\x1b[0m", row, text))
	r.sgrFg = -1
	r.sgrBg = -1
}

func (r *AnsiRenderer) End() {
	r.io.Write("\x1b[?1006l\x1b[?1003l\x1b[?1002l\x1b[?1000l" +
		"\x1b[0m\x1b[?25h\x1b[?7h\x1b[2J\x1b[H")
}

func (r *AnsiRenderer) sgrStr(fg, bg int32) string {
	var out strings.Builder
	if fg != r.sgrFg {
		fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm", (fg>>16)&0xff, (fg>>8)&0xff, fg&0xff)
		r.sgrFg = fg
	}
	if bg != r.sgrBg {
		fmt.Fprintf(&out, "\x1b[48;2;%d;%d;%dm", (bg>>16)&0xff, (bg>>8)&0xff, bg&0xff)
		r.sgrBg = bg
	}
	return out.String()
}

func averageColor(pixels []byte, x0, y0, x1, y1, stride int) int32 {
	var sumB, sumG, sumR, count int
	for y := y0; y < y1; y++ {
		offset := (y*stride + x0) * 4
		for x := x0; x < x1; x++ {
			sumB += int(pixels[offset])
			sumG += int(pixels[offset+1])
			sumR += int(pixels[offset+2])
			count++
			offset += 4
		}
	}
	if count == 0 {
		return 0
	}
	r := (sumR + count/2) / count
	g := (sumG + count/2) / count
	b := (sumB + count/2) / count
	return int32((r << 16) | (g << 8) | b)
}
