package compress

import "ghostwire/internal/domain"

type NullCompressor struct{}

func (c *NullCompressor) Name() string {
	return "none"
}

func (c *NullCompressor) Compress(data []byte) ([]byte, error) {
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

func (c *NullCompressor) Decompress(_ []byte, _ int) ([]byte, error) {
	return nil, domain.NewError(domain.ErrInternal, "this compressor does not decompress")
}
