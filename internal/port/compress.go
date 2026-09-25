package port

type CompressorPort interface {
	Name() string
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte, expectedLength int) ([]byte, error)
}
