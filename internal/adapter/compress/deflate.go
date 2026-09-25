package compress

import (
	"bytes"
	"compress/flate"
	"io"

	"ghostwire/internal/domain"
)

type DeflateCompressor struct {
	level int
}

func NewDeflateCompressor(level int) *DeflateCompressor {
	if level == 0 {
		level = 1
	}
	return &DeflateCompressor{level: level}
}

func (c *DeflateCompressor) Name() string {
	return "deflate"
}

func (c *DeflateCompressor) Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, c.level)
	if err != nil {
		return nil, domain.WrapError(err, domain.ErrInternal, "failed to create deflate writer")
	}
	if _, err := w.Write(data); err != nil {
		w.Close()
		return nil, domain.WrapError(err, domain.ErrInternal, "deflate write failed")
	}
	if err := w.Close(); err != nil {
		return nil, domain.WrapError(err, domain.ErrInternal, "deflate close failed")
	}
	return buf.Bytes(), nil
}

func (c *DeflateCompressor) Decompress(data []byte, expectedLength int) ([]byte, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		return nil, domain.WrapError(err, domain.ErrProtocol, "inflate failed")
	}
	if len(out) != expectedLength {
		return nil, domain.NewError(domain.ErrProtocol,
			"inflated size does not match expected")
	}
	return out, nil
}
