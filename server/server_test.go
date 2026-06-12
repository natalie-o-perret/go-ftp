package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/natalie-o-perret/go-ftp/ftp"
	"github.com/natalie-o-perret/go-ftp/server/auth"
	"github.com/natalie-o-perret/go-ftp/server/fs"
)

type ftpClient struct {
	c   net.Conn
	br  *bufio.Reader
	tag string
}

func dial(t *testing.T, addr string) *ftpClient {
	t.Helper()
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	cl := &ftpClient{c: c, br: bufio.NewReader(c)}
	cl.readReply(t, 220)
	return cl
}

func (c *ftpClient) readReply(t *testing.T, wantCode int) ftp.Reply {
	t.Helper()
	reply, err := ftp.NewReplyScanner(c.br).Read()
	if err != nil {
		t.Fatal(err)
	}
	if reply.Code != ftp.Code(wantCode) {
		t.Fatalf("got %d %s, want %d", reply.Code, reply.Text, wantCode)
	}
	return reply
}

func (c *ftpClient) send(t *testing.T, name, arg string) {
	t.Helper()
	if _, err := fmt.Fprintf(c.c, "%s %s\r\n", name, arg); err != nil {
		t.Fatal(err)
	}
}

func (c *ftpClient) close() { c.c.Close() }

func TestServerGreetAndQuit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv, err := New(Config{
		Name:        "test",
		Listen:      ln.Addr().String(),
		Authenticator: auth.NewStatic(),
		Filesystem:  fs.NewOS(t.TempDir()),
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.Shutdown(testCtx(t))

	cl := dial(t, ln.Addr().String())
	defer cl.close()
	cl.send(t, "QUIT", "")
	cl.readReply(t, 221)
}

func TestServerLoginAndList(t *testing.T) {
	root := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	st := auth.NewStatic()
	st.Add("alice", "hunter2")
	srv, err := New(Config{
		Name:          "test",
		Listen:        ln.Addr().String(),
		Authenticator: st,
		Filesystem:    fs.NewOS(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.Shutdown(testCtx(t))

	cl := dial(t, ln.Addr().String())
	defer cl.close()
	cl.send(t, "USER", "alice")
	cl.readReply(t, 331)
	cl.send(t, "PASS", "hunter2")
	cl.readReply(t, 230)

	cl.send(t, "PWD", "")
	cl.readReply(t, 257)
	cl.send(t, "CWD", "/")
	cl.readReply(t, 250)
}

func TestServerPassiveRetrieve(t *testing.T) {
	root := t.TempDir()
	want := []byte("hello ftp world\n")
	if err := os.WriteFile(filepath.Join(root, "greet.txt"), want, 0o644); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv, err := New(Config{
		Name:          "test",
		Listen:        ln.Addr().String(),
		Authenticator: auth.AllowAnonymous(),
		Filesystem:    fs.NewOS(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve(ln)
	defer srv.Shutdown(testCtx(t))

	cl := dial(t, ln.Addr().String())
	defer cl.close()
	cl.send(t, "USER", "anonymous")
	cl.readReply(t, 331)
	cl.send(t, "PASS", "x@x")
	cl.readReply(t, 230)

	cl.send(t, "TYPE", "I")
	cl.readReply(t, 200)
	cl.send(t, "EPSV", "")
	r := cl.readReply(t, 229)
	port, err := ftp.ParseEPSVReply(r)
	if err != nil {
		t.Fatal(err)
	}
	host, _, _ := net.SplitHostPort(ln.Addr().String())
	data, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	cl.send(t, "RETR", "greet.txt")
	cl.readReply(t, 150)
	got, err := io.ReadAll(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), string(want)) {
		t.Fatalf("got %q want %q", got, want)
	}
	cl.readReply(t, 226)
}

func testCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
