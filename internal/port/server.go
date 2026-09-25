package port

import "context"

type ServeOptions struct {
	Address            string
	Token              string
	Fps                int
	MaxWidth           int
	Compress           string
	HandshakeTimeoutMs int
	PingIntervalMs     int
	PingTimeoutMs      int
	Tls                *TlsListenOptions
}

type ConnectOptions struct {
	Address            string
	Token              string
	ClientName         string
	HandshakeTimeoutMs int
	PingIntervalMs     int
	PingTimeoutMs      int
	Tls                *TlsDialOptions
}

type TlsListenOptions struct {
	CertPath string
	KeyPath  string
}

type TlsDialOptions struct {
	CaCertPath string
	Insecure   bool
}

type ServerStats struct {
	ConnectionsAccepted int
	SessionsServed      int
	FramesSent          int
	FramesSkipped       int
	InputEvents         int
	BytesSent           int
}

type ServerPort interface {
	Serve(opts ServeOptions, ctx context.Context) error
	Stats() ServerStats
}
