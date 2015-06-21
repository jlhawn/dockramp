package build

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

func (b *Builder) hijack(method, path string, in io.Reader, out io.Writer, started chan int) error {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		return fmt.Errorf("unable to create hijack request: %s", err)
	}

	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	req.Host = b.client.URL.Host

	var (
		conn    net.Conn
		dialErr error
	)

	u, err := url.Parse(b.daemonURL)
	if err != nil {
		return fmt.Errorf("unable to parse daemon URL: %s", err)
	}

	switch u.Scheme {
	case "unix":
		socketPath := u.Path
		conn, dialErr = net.Dial("unix", socketPath)
	default:
		if b.tlsConfig == nil {
			conn, dialErr = net.Dial("tcp", u.Host)
		} else {
			conn, dialErr = tlsDial("tcp", u.Host, b.tlsConfig)
		}
	}

	if dialErr != nil {
		return fmt.Errorf("unable to dial for hijack: %s", dialErr)
	}

	// When we set up a TCP connection for hijack, there could be long periods
	// of inactivity (a long running command with no output) that in certain
	// network setups may cause ECONNTIMEOUT, leaving the client in an unknown
	// state. Setting TCP KeepAlive on the socket connection will prohibit
	// ECONNTIMEOUT unless the socket connection truly is broken
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	clientconn := httputil.NewClientConn(conn, nil)
	defer clientconn.Close()

	// Server hijacks the connection, error 'connection closed' expected
	clientconn.Do(req)

	rwc, br := clientconn.Hijack()
	defer rwc.Close()

	started <- 1

	outputErr := make(chan error, 1)
	inputErr := make(chan error, 1)

	// Spawn a goroutine to copy hijacked output to the output stream.
	go func() {
		_, err = io.Copy(out, br)
		outputErr <- err
	}()

	// Spawn a goroutine to copy input into the hijacked stream.
	go func() {
		io.Copy(rwc, in)

		if cw, ok := rwc.(interface {
			CloseWrite() error
		}); ok {
			cw.CloseWrite()
		} else {
			log.Fatal("unable to close write end of stream")
		}

		// Discard errors due to pipe interruption.
		inputErr <- nil
	}()

	if err := <-outputErr; err != nil {
		return fmt.Errorf("unable to get output: %s", err)
	}

	if err := <-inputErr; err != nil {
		return fmt.Errorf("unable to send input: %s", err)
	}

	return nil
}

/******************************************************
 * Hack needed to close write-end of hijacked stream. *
 ******************************************************/

type tlsClientConn struct {
	*tls.Conn
	rawConn net.Conn
}

func (c *tlsClientConn) CloseWrite() error {
	// Go standard tls.Conn doesn't provide the CloseWrite() method so we do it
	// on its underlying connection.
	if cwc, ok := c.rawConn.(interface {
		CloseWrite() error
	}); ok {
		return cwc.CloseWrite()
	}
	return nil
}

func tlsDial(network, addr string, config *tls.Config) (net.Conn, error) {
	return tlsDialWithDialer(new(net.Dialer), network, addr, config)
}

// We need to copy Go's implementation of tls.Dial (pkg/cryptor/tls/tls.go) in
// order to return our custom tlsClientConn struct which holds both the tls.Conn
// object _and_ its underlying raw connection. The rationale for this is that
// we need to be able to close the write end of the connection when attaching,
// which tls.Conn does not provide.
func tlsDialWithDialer(dialer *net.Dialer, network, addr string, config *tls.Config) (net.Conn, error) {
	// We want the Timeout and Deadline values from dialer to cover the
	// whole process: TCP connection and TLS handshake. This means that we
	// also need to start our own timers now.
	timeout := dialer.Timeout

	if !dialer.Deadline.IsZero() {
		deadlineTimeout := dialer.Deadline.Sub(time.Now())
		if timeout == 0 || deadlineTimeout < timeout {
			timeout = deadlineTimeout
		}
	}

	var errChannel chan error

	if timeout != 0 {
		errChannel = make(chan error, 2)
		time.AfterFunc(timeout, func() {
			errChannel <- errors.New("")
		})
	}

	rawConn, err := dialer.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	// When we set up a TCP connection for hijack, there could be long periods
	// of inactivity (a long running command with no output) that in certain
	// network setups may cause ECONNTIMEOUT, leaving the client in an unknown
	// state. Setting TCP KeepAlive on the socket connection will prohibit
	// ECONNTIMEOUT unless the socket connection truly is broken
	if tcpConn, ok := rawConn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	hostname := addr[:colonPos]

	// If no ServerName is set, infer the ServerName
	// from the hostname we're connecting to.
	if config.ServerName == "" {
		// Make a copy to avoid polluting argument or default.
		c := *config
		c.ServerName = hostname
		config = &c
	}

	conn := tls.Client(rawConn, config)

	if timeout == 0 {
		err = conn.Handshake()
	} else {
		go func() {
			errChannel <- conn.Handshake()
		}()

		err = <-errChannel
	}

	if err != nil {
		rawConn.Close()
		return nil, err
	}

	// This is Docker difference with standard's crypto/tls package: returned a
	// wrapper which holds both the TLS and raw connections.
	return &tlsClientConn{conn, rawConn}, nil
}
