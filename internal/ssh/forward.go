package ssh

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cssh "golang.org/x/crypto/ssh"
)

const keepAliveInterval = 30 * time.Second

// PortForward is a local (-L style) port forward: connections accepted on
// Local are tunneled through the SSH host and dialed to Remote from there.
// It stays open until Close is called, reconnecting the SSH client if the
// connection drops.
type PortForward struct {
	Host   string
	Local  string // actual listening address
	Remote string
	Spec   Forward // what was asked for, as saved in config

	cfg      SSHConfig
	listener net.Listener
	active   atomic.Int64

	mu     sync.Mutex
	client *cssh.Client
	closed bool
	done   chan struct{}
}

// Forward is a saved local port forward, with both sides normalized.
type Forward struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

func NewForward(local, remote string) Forward {
	return Forward{Local: NormalizeLocalAddr(local), Remote: NormalizeRemoteAddr(remote)}
}

// ParseLocalForward parses the value of an ssh_config LocalForward line:
// "[bind_address:]port host:hostport".
func ParseLocalForward(value string) (Forward, bool) {
	fields := strings.Fields(value)
	if len(fields) != 2 {
		return Forward{}, false
	}
	return NewForward(fields[0], fields[1]), true
}

// ConfigValue renders f as an ssh_config LocalForward value, omitting the
// default 127.0.0.1 bind address like OpenSSH users usually write it.
func (f Forward) ConfigValue() string {
	return strings.TrimPrefix(f.Local, "127.0.0.1:") + " " + f.Remote
}

func (f Forward) String() string {
	return strings.TrimPrefix(f.Local, "127.0.0.1:") + " → " + f.Remote
}

// ParseLFlag parses ssh -L syntax: "[bind:]port:host:hostport", or a lone
// "port" for the same port on the remote host's localhost.
func ParseLFlag(s string) (Forward, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	switch len(parts) {
	case 1:
		return NewForward(parts[0], parts[0]), validPort(parts[0], s)
	case 3, 4:
		n := len(parts)
		local := strings.Join(parts[:n-2], ":")
		_, lport, _ := strings.Cut(NormalizeLocalAddr(local), ":")
		if err := validPort(lport, s); err != nil {
			return Forward{}, err
		}
		if err := validPort(parts[n-1], s); err != nil || parts[n-2] == "" {
			return Forward{}, fmt.Errorf("invalid forward %q, expected [bind:]port:host:hostport", s)
		}
		return NewForward(local, parts[n-2]+":"+parts[n-1]), nil
	}
	return Forward{}, fmt.Errorf("invalid forward %q, expected [bind:]port:host:hostport", s)
}

func validPort(p, in string) error {
	if n, err := strconv.Atoi(p); err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("invalid port %q in forward %q", p, in)
	}
	return nil
}

// LFlag renders f in ssh -L syntax, the inverse of ParseLFlag.
func (f Forward) LFlag() string {
	return strings.TrimPrefix(f.Local, "127.0.0.1:") + ":" + f.Remote
}

// NormalizeLocalAddr turns "8080" into "127.0.0.1:8080" and leaves
// "host:port" untouched.
func NormalizeLocalAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if !strings.Contains(addr, ":") {
		return "127.0.0.1:" + addr
	}
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}

// NormalizeRemoteAddr turns "80" into "localhost:80" and leaves "host:port"
// untouched.
func NormalizeRemoteAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if !strings.Contains(addr, ":") {
		return "localhost:" + addr
	}
	if strings.HasPrefix(addr, ":") {
		return "localhost" + addr
	}
	return addr
}

func StartPortForward(cfg SSHConfig, spec Forward) (*PortForward, error) {
	spec = NewForward(spec.Local, spec.Remote)
	local, remote := spec.Local, spec.Remote
	if _, _, err := net.SplitHostPort(local); err != nil {
		return nil, fmt.Errorf("invalid local address %q: %w", local, err)
	}
	if _, _, err := net.SplitHostPort(remote); err != nil {
		return nil, fmt.Errorf("invalid remote address %q: %w", remote, err)
	}
	client, err := SSHClient(cfg)
	if err != nil {
		return nil, err
	}
	l, err := net.Listen("tcp", local)
	if err != nil {
		client.Close()
		return nil, err
	}
	f := &PortForward{
		Host:     cfg.Host,
		Local:    l.Addr().String(),
		Remote:   remote,
		Spec:     spec,
		cfg:      cfg,
		listener: l,
		client:   client,
		done:     make(chan struct{}),
	}
	go f.keepAlive()
	go f.serve()
	return f, nil
}

func (f *PortForward) String() string {
	return fmt.Sprintf("%s → %s:%s", f.Local, f.Host, f.Remote)
}

// ActiveConns is the number of currently tunneled connections.
func (f *PortForward) ActiveConns() int64 { return f.active.Load() }

func (f *PortForward) Close() error {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	close(f.done)
	client := f.client
	f.client = nil
	f.mu.Unlock()

	err := f.listener.Close()
	if client != nil {
		client.Close()
	}
	return err
}

func (f *PortForward) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go f.handle(conn)
	}
}

func (f *PortForward) handle(local net.Conn) {
	defer local.Close()
	remote, err := f.dial()
	if err != nil {
		return
	}
	defer remote.Close()

	f.active.Add(1)
	defer f.active.Add(-1)

	var wg sync.WaitGroup
	wg.Add(2)
	pipe := func(dst, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			cw.CloseWrite()
		} else {
			dst.Close()
		}
	}
	go pipe(remote, local)
	go pipe(local, remote)
	wg.Wait()
}

// dial opens a channel to Remote, reconnecting the SSH client once if the
// current one is dead.
func (f *PortForward) dial() (net.Conn, error) {
	client, err := f.getClient()
	if err != nil {
		return nil, err
	}
	conn, err := client.Dial("tcp", f.Remote)
	if err == nil {
		return conn, nil
	}
	if ping(client) {
		// SSH link is fine, the remote target itself refused.
		return nil, err
	}
	f.dropClient(client)
	if client, err = f.getClient(); err != nil {
		return nil, err
	}
	return client.Dial("tcp", f.Remote)
}

func (f *PortForward) getClient() (*cssh.Client, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return nil, net.ErrClosed
	}
	if f.client != nil {
		return f.client, nil
	}
	client, err := SSHClient(f.cfg)
	if err != nil {
		return nil, err
	}
	f.client = client
	return client, nil
}

func (f *PortForward) dropClient(c *cssh.Client) {
	f.mu.Lock()
	if f.client == c {
		f.client = nil
	}
	f.mu.Unlock()
	c.Close()
}

// keepAlive pings the SSH server so idle tunnels are not dropped by
// NATs/firewalls, and discards the client as soon as it is found dead so the
// next connection reconnects.
func (f *PortForward) keepAlive() {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-f.done:
			return
		case <-ticker.C:
		}
		f.mu.Lock()
		client := f.client
		f.mu.Unlock()
		if client == nil {
			continue
		}
		if !ping(client) {
			f.dropClient(client)
		}
	}
}

// ping reports whether the SSH server answers a keepalive in time; a
// silently dead TCP link would otherwise block forever.
func ping(c *cssh.Client) bool {
	res := make(chan error, 1)
	go func() {
		_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
		res <- err
	}()
	select {
	case err := <-res:
		return err == nil
	case <-time.After(15 * time.Second):
		return false
	}
}
