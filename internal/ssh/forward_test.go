package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"testing"

	cssh "golang.org/x/crypto/ssh"
)

// startTestSSHServer runs a minimal SSH server that accepts password "pw"
// and serves direct-tcpip channels (what -L forwarding uses).
func startTestSSHServer(t *testing.T) (host, port string) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := cssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &cssh.ServerConfig{
		PasswordCallback: func(_ cssh.ConnMetadata, pw []byte) (*cssh.Permissions, error) {
			if string(pw) == "pw" {
				return nil, nil
			}
			return nil, io.EOF
		},
	}
	cfg.AddHostKey(signer)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := cssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				go cssh.DiscardRequests(reqs)
				for nc := range chans {
					if nc.ChannelType() != "direct-tcpip" {
						nc.Reject(cssh.UnknownChannelType, "")
						continue
					}
					d := nc.ExtraData()
					hl := binary.BigEndian.Uint32(d)
					target := net.JoinHostPort(string(d[4:4+hl]),
						strconv.Itoa(int(binary.BigEndian.Uint32(d[4+hl:]))))
					dst, err := net.Dial("tcp", target)
					if err != nil {
						nc.Reject(cssh.ConnectionFailed, err.Error())
						continue
					}
					ch, chReqs, _ := nc.Accept()
					go cssh.DiscardRequests(chReqs)
					go func() { io.Copy(dst, ch); dst.Close() }()
					go func() { io.Copy(ch, dst); ch.Close() }()
				}
			}()
		}
	}()
	h, p, _ := net.SplitHostPort(l.Addr().String())
	return h, p
}

func TestPortForward(t *testing.T) {
	// Echo server reachable "from the remote host".
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	host, port := startTestSSHServer(t)
	cfg := SSHConfig{Host: "test", HostName: host, Port: port, User: "u", Password: "pw"}
	f, err := StartPortForward(cfg, NewForward("0", echo.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	roundTrip := func() {
		t.Helper()
		c, err := net.Dial("tcp", f.Local)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if _, err := c.Write([]byte("hello")); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "hello" {
			t.Fatalf("got %q, %v", buf, err)
		}
	}
	roundTrip()

	// Kill the SSH connection: the forward must reconnect and keep working.
	f.mu.Lock()
	f.client.Close()
	f.mu.Unlock()
	roundTrip()

	f.Close()
	if _, err := net.Dial("tcp", f.Local); err == nil {
		t.Fatal("listener still open after Close")
	}
}

func TestNormalizeAddrs(t *testing.T) {
	for in, want := range map[string]string{"8080": "127.0.0.1:8080", ":8080": "127.0.0.1:8080", "0.0.0.0:80": "0.0.0.0:80"} {
		if got := NormalizeLocalAddr(in); got != want {
			t.Errorf("local %q: got %q want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"80": "localhost:80", "db:5432": "db:5432"} {
		if got := NormalizeRemoteAddr(in); got != want {
			t.Errorf("remote %q: got %q want %q", in, got, want)
		}
	}
}

func TestParseLFlag(t *testing.T) {
	for in, want := range map[string]Forward{
		"9000":                 {"127.0.0.1:9000", "localhost:9000"},
		"8080:localhost:80":    {"127.0.0.1:8080", "localhost:80"},
		"0.0.0.0:9000:db:9000": {"0.0.0.0:9000", "db:9000"},
		" 5432:10.0.0.5:5432 ": {"127.0.0.1:5432", "10.0.0.5:5432"},
	} {
		got, err := ParseLFlag(in)
		if err != nil || got != want {
			t.Errorf("%q: got %+v, %v", in, got, err)
		}
		if back, _ := ParseLFlag(got.LFlag()); back != got {
			t.Errorf("%q: LFlag round trip gave %+v", in, back)
		}
	}
	for _, bad := range []string{"", "abc", "80:host", "80:host:x", "a:b:c:d:e"} {
		if _, err := ParseLFlag(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
