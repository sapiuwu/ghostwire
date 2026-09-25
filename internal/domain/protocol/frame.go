package protocol

import (
	"encoding/binary"
	"fmt"

	"ghostwire/internal/domain"
)

type FrameMeta struct {
	Seq         uint32
	TimestampMs uint64
	X           uint16
	Y           uint16
	Width       uint16
	Height      uint16
	PixelFormat uint8
	Encoding    uint8
	RawLength   uint32
}

func EncodeFrameMeta(meta FrameMeta) []byte {
	buf := make([]byte, FrameMetaSize)
	binary.BigEndian.PutUint32(buf[0:4], meta.Seq)
	binary.BigEndian.PutUint64(buf[4:12], meta.TimestampMs)
	binary.BigEndian.PutUint16(buf[12:14], meta.X)
	binary.BigEndian.PutUint16(buf[14:16], meta.Y)
	binary.BigEndian.PutUint16(buf[16:18], meta.Width)
	binary.BigEndian.PutUint16(buf[18:20], meta.Height)
	buf[20] = meta.PixelFormat
	buf[21] = meta.Encoding
	binary.BigEndian.PutUint32(buf[22:26], meta.RawLength)
	return buf
}

func DecodeFrameMeta(data []byte, offset int) (FrameMeta, error) {
	if len(data)-offset < FrameMetaSize {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol, "truncated frame header")
	}
	s := offset
	meta := FrameMeta{
		Seq:         binary.BigEndian.Uint32(data[s : s+4]),
		TimestampMs: binary.BigEndian.Uint64(data[s+4 : s+12]),
		X:           binary.BigEndian.Uint16(data[s+12 : s+14]),
		Y:           binary.BigEndian.Uint16(data[s+14 : s+16]),
		Width:       binary.BigEndian.Uint16(data[s+16 : s+18]),
		Height:      binary.BigEndian.Uint16(data[s+18 : s+20]),
		PixelFormat: data[s+20],
		Encoding:    data[s+21],
		RawLength:   binary.BigEndian.Uint32(data[s+22 : s+26]),
	}
	if meta.PixelFormat != uint8(PixelBGRA8) {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("unsupported pixel format %d", meta.PixelFormat))
	}
	if meta.Encoding != uint8(FrameRaw) && meta.Encoding != uint8(FrameDeflate) {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("unsupported frame encoding %d", meta.Encoding))
	}
	if meta.Width <= 0 || meta.Height <= 0 {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol, "invalid frame dimensions")
	}
	expected := uint32(meta.Width) * uint32(meta.Height) * 4
	if meta.RawLength != expected {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol,
			fmt.Sprintf("frame raw length %d does not match %d", meta.RawLength, expected))
	}
	if meta.RawLength > uint32(MaxFrameRawSize) {
		return FrameMeta{}, domain.NewError(domain.ErrProtocol, "frame raw length exceeds limit")
	}
	return meta, nil
}


