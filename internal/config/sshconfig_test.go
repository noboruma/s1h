package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/noboruma/s1h/internal/ssh"
)

const sample = `# my config
Host web
  HostName 10.0.0.1
  User root
  IdentityFile ~/.ssh/web
  LocalForward 5432 localhost:5432

# defaults
Host *
  User nobody
`

func setup(t *testing.T) (link, real string) {
	t.Helper()
	dir := t.TempDir()
	real = filepath.Join(dir, "real_config")
	if err := os.WriteFile(real, []byte(sample), 0o640); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "config")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	return link, real
}

func check(t *testing.T, path, want string) {
	t.Helper()
	got, _ := os.ReadFile(path)
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestUpsertExistingHost(t *testing.T) {
	path, real := setup(t)
	err := UpsertHost(path, HostBlock{Host: "web", HostName: "10.0.0.9", Port: "2222", User: "admin",
		LocalForwards: []ssh.Forward{ssh.NewForward("8080", "localhost:80"), ssh.NewForward("0.0.0.0:9000", "db:9000")}})
	if err != nil {
		t.Fatal(err)
	}
	check(t, path, `# my config
Host web
  HostName 10.0.0.9
  User admin
  IdentityFile ~/.ssh/web
  LocalForward 8080 localhost:80
  LocalForward 0.0.0.0:9000 db:9000
  Port 2222

# defaults
Host *
  User nobody
`)
	if fi, _ := os.Lstat(path); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink was replaced")
	}
	if fi, _ := os.Stat(real); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode changed to %v", fi.Mode().Perm())
	}

	// Clearing forwards removes the lines.
	if err := UpsertHost(path, HostBlock{Host: "web", HostName: "10.0.0.9", Port: "2222", User: "admin"}); err != nil {
		t.Fatal(err)
	}
	cfgs, _ := ssh.ParseSSHConfig(path)
	if len(cfgs) != 1 || len(cfgs[0].LocalForwards) != 0 || cfgs[0].User != "admin" || cfgs[0].IdentityFile != "~/.ssh/web" {
		t.Fatalf("parsed %+v", cfgs)
	}
}

func TestAddAndRemoveHost(t *testing.T) {
	path, _ := setup(t)
	if err := UpsertHost(path, HostBlock{Host: "new", HostName: "10.0.0.2", Port: "22", User: "me",
		LocalForwards: []ssh.Forward{ssh.NewForward("3000", "localhost:3000")}}); err != nil {
		t.Fatal(err)
	}
	check(t, path, `# my config
Host web
  HostName 10.0.0.1
  User root
  IdentityFile ~/.ssh/web
  LocalForward 5432 localhost:5432

Host new
    HostName 10.0.0.2
    Port 22
    User me
    LocalForward 3000 localhost:3000

# defaults
Host *
  User nobody
`)
	cfgs, _ := ssh.ParseSSHConfig(path)
	if len(cfgs) != 2 || cfgs[1].Host != "new" || cfgs[1].User != "me" {
		t.Fatalf("parsed %+v", cfgs)
	}

	if err := RemoveHost(path, "new"); err != nil {
		t.Fatal(err)
	}
	check(t, path, sample)
	if err := RemoveHost(path, "nope"); err == nil {
		t.Error("expected error")
	}
}

func TestRemoveLastHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	orig := "Host a\n  HostName x\n"
	os.WriteFile(path, []byte(orig), 0o600)
	if err := UpsertHost(path, HostBlock{Host: "b", HostName: "y"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveHost(path, "b"); err != nil {
		t.Fatal(err)
	}
	check(t, path, orig)
}

func TestUpsertCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh", "config")
	if err := UpsertHost(path, HostBlock{Host: "a", HostName: "h", Port: "22"}); err != nil {
		t.Fatal(err)
	}
	check(t, path, "Host a\n    HostName h\n    Port 22\n")
}
