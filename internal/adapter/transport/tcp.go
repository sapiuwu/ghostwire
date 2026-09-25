package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"ghostwire/internal/domain"
	"ghostwire/internal/port"
)

const (
	connectTimeoutMs = 10000
	closeLingerMs    = 2000
)

func ParseAddress(address string) (string, int, error) {
	idx := strings.LastIndex(address, ":")
	if idx < 0 {
		return "", 0, domain.NewError(domain.ErrTransport,
			fmt.Sprintf("invalid address %q (expected host:port)", address))
	}
	host := address[:idx]
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	portStr := address[idx+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, domain.NewError(domain.ErrTransport,
			fmt.Sprintf("invalid port in address %q", address))
	}
	return host, port, nil
}

type tcpConn struct {
	id            string
	remoteAddress string
	socket        net.Conn
	closing       bool
	mu            sync.Mutex
	dataCb        func([]byte)
	errorCb       func(error)
	closeCb       func()
	drainCb       func()
}

func (c *tcpConn) ID() string          { return c.id }
func (c *tcpConn) RemoteAddress() string { return c.remoteAddress }

func (c *tcpConn) Write(chunk []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing {
		return false
	}
	_, err := c.socket.Write(chunk)
	return err == nil
}

func (c *tcpConn) OnData(cb func([]byte))  { c.mu.Lock(); c.dataCb = cb; c.mu.Unlock() }
func (c *tcpConn) OnError(cb func(error))  { c.mu.Lock(); c.errorCb = cb; c.mu.Unlock() }
func (c *tcpConn) OnClose(cb func())       { c.mu.Lock(); c.closeCb = cb; c.mu.Unlock() }
func (c *tcpConn) OnDrain(cb func())       { c.mu.Lock(); c.drainCb = cb; c.mu.Unlock() }

func (c *tcpConn) Close() {
	c.mu.Lock()
	if c.closing {
		c.mu.Unlock()
		return
	}
	c.closing = true
	c.mu.Unlock()
	c.socket.Close()
}

func (c *tcpConn) readLoop() {
	buf := make([]byte, 65536)
	for {
		n, err := c.socket.Read(buf)
		if n > 0 {
			c.mu.Lock()
			cb := c.dataCb
			c.mu.Unlock()
			if cb != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				cb(data)
			}
		}
		if err != nil {
			c.mu.Lock()
			errCb := c.errorCb
			closeCb := c.closeCb
			c.mu.Unlock()
			if err != io.EOF && errCb != nil {
				errCb(err)
			}
			if closeCb != nil {
				closeCb()
			}
			return
		}
	}
}

type tcpListener struct {
	address      string
	server       net.Listener
	connCb       func(port.Conn)
	errorCb      func(error)
	conns        sync.Map
	closed       bool
	mu           sync.Mutex
}

func (l *tcpListener) Address() string { return l.address }

func (l *tcpListener) OnConnection(cb func(port.Conn)) { l.connCb = cb }
func (l *tcpListener) OnError(cb func(error))           { l.errorCb = cb }

func (l *tcpListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.mu.Unlock()
	return l.server.Close()
}

func (l *tcpListener) acceptLoop() {
	for {
		socket, err := l.server.Accept()
		if err != nil {
			l.mu.Lock()
			closed := l.closed
			errCb := l.errorCb
			l.mu.Unlock()
			if closed {
				return
			}
			if errCb != nil {
				errCb(err)
			}
			return
		}
		conn := newTCPConn(socket)
		l.conns.Store(conn.id, conn)
		go func() {
			<-func() chan struct{} {
				ch := make(chan struct{})
				conn.OnClose(func() {
					l.conns.Delete(conn.id)
					close(ch)
				})
				return ch
			}()
		}()
		if l.connCb != nil {
			l.connCb(conn)
		}
	}
}

var connSeq int

func newTCPConn(socket net.Conn) *tcpConn {
	connSeq++
	id := fmt.Sprintf("conn-%d", connSeq)
	host := socket.RemoteAddr().(*net.TCPAddr).IP.String()
	port := socket.RemoteAddr().(*net.TCPAddr).Port
	return &tcpConn{
		id:            id,
		remoteAddress: fmt.Sprintf("%s:%d", host, port),
		socket:        socket,
	}
}

type TCPTransport struct{}

func (t *TCPTransport) Listen(address string, tlsOpts *port.TlsListenOptions) (port.Listener, error) {
	host, portNum, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}

	var listener net.Listener
	addr := fmt.Sprintf("%s:%d", host, portNum)
	if host == "" {
		addr = fmt.Sprintf(":%d", portNum)
	}

	if tlsOpts != nil {
		cert, err := tls.LoadX509KeyPair(tlsOpts.CertPath, tlsOpts.KeyPath)
		if err != nil {
			return nil, domain.WrapError(err, domain.ErrTransport, "failed to load TLS credentials")
		}
		config := &tls.Config{Certificates: []tls.Certificate{cert}}
		listener, err = tls.Listen("tcp", addr, config)
		if err != nil {
			return nil, domain.WrapError(err, domain.ErrTransport, "failed to listen")
		}
	} else {
		listener, err = net.Listen("tcp", addr)
		if err != nil {
			return nil, domain.WrapError(err, domain.ErrTransport, "failed to listen")
		}
	}

	tcpL := &tcpListener{
		address: listener.Addr().String(),
		server:  listener,
	}
	go tcpL.acceptLoop()
	return tcpL, nil
}

func (t *TCPTransport) Dial(address string, tlsOpts *port.TlsDialOptions) (port.Conn, error) {
	host, portNum, err := ParseAddress(address)
	if err != nil {
		return nil, err
	}
	if host == "" {
		host = "127.0.0.1"
	}

	dialer := &net.Dialer{Timeout: connectTimeoutMs * time.Millisecond}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", portNum))

	var rawConn net.Conn
	if tlsOpts != nil {
		config := &tls.Config{
			InsecureSkipVerify: tlsOpts.Insecure,
		}
		if tlsOpts.CaCertPath != "" {
			caCert, err := os.ReadFile(tlsOpts.CaCertPath)
			if err != nil {
				return nil, domain.WrapError(err, domain.ErrTransport, "failed to read CA certificate")
			}
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			config.RootCAs = pool
		}
		rawConn, err = tls.DialWithDialer(dialer, "tcp", addr, config)
	} else {
		rawConn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return nil, domain.WrapError(err, domain.ErrTransport,
			fmt.Sprintf("failed to connect to %s", address))
	}

	conn := newTCPConn(rawConn)
	go conn.readLoop()
	return conn, nil
}
