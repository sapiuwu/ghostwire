package port

type Conn interface {
	ID() string
	RemoteAddress() string
	Write(chunk []byte) bool
	OnData(cb func(chunk []byte))
	OnError(cb func(err error))
	OnClose(cb func())
	OnDrain(cb func())
	Close()
}

type Listener interface {
	Address() string
	OnConnection(cb func(conn Conn))
	OnError(cb func(err error))
	Close() error
}

type TransportPort interface {
	Listen(address string, tls *TlsListenOptions) (Listener, error)
	Dial(address string, tls *TlsDialOptions) (Conn, error)
}
