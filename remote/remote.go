// Package remote reaches other machines over ssh: it runs commands there,
// for what ksesh asks about an agent or a directory on another machine (its
// branch, its review, the directories the host's zoxide knows), and builds
// the `kitten ssh` command line a kitty window opens one with.
//
// A window ksesh opens on a host forwards kitty's remote control socket
// (the ssh kitten's forward_remote_control), and tells the host its name
// and the window's id, so the hooks of an agent started there report to
// this kitty, under the name ssh knows the host by.
package remote

import (
	"fmt"
	"path"
	"strings"

	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/shell"
)

// Remote runs commands on other machines.
type Remote interface {
	// Hosts are the configured [[host]]s.
	Hosts() []model.HostConfig
	// Run runs a command on host, in dir when given, in a login shell, so
	// the PATH is the one you get logging in, and returns what it printed.
	Run(host, dir string, argv ...string) (string, error)
	// Dirs are the directories to list on host: its configured paths, then
	// those its zoxide knows, with its home directory.
	Dirs(host string) (home string, dirs []string, err error)
}

// HostVar is the environment variable a window ksesh opens on a host
// names it in, for the agents' hooks there (see `ksesh agent hook`).
const HostVar = "KSESH_HOST"

type RealRemote struct {
	shell shell.Shell
	hosts []model.HostConfig
}

func NewRemote(sh shell.Shell, hosts []model.HostConfig) Remote {
	return &RealRemote{shell: sh, hosts: hosts}
}

func (r *RealRemote) Hosts() []model.HostConfig { return r.hosts }

// sshOptions keep a command from waiting on a password or a host key
// question nobody will see, or on a host that doesn't answer.
var sshOptions = []string{"-o", "BatchMode=yes", "-o", "ConnectTimeout=5"}

func (r *RealRemote) Run(host, dir string, argv ...string) (string, error) {
	script := Join(argv...)
	if dir != "" {
		script = "cd " + ShellDir(dir) + " && " + script
	}
	args := append(append([]string{}, sshOptions...), host, LoginShell(script))
	out, err := r.shell.CmdCapture("ssh", args...)
	if err != nil {
		return out, fmt.Errorf("on %s: %w", host, err)
	}
	return out, nil
}

func (r *RealRemote) Dirs(host string) (string, []string, error) {
	cfg, _ := r.host(host)
	script := `printf '%s\n' "$HOME"`
	if cfg.Zoxide == nil || *cfg.Zoxide {
		script += "; command -v zoxide >/dev/null && zoxide query -l"
	}
	out, err := r.shell.CmdCapture("ssh", append(append([]string{}, sshOptions...), host, LoginShell(script))...)
	if err != nil {
		return "", nil, fmt.Errorf("on %s: %w", host, err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	home := strings.TrimSpace(lines[0])
	var dirs []string
	seen := map[string]bool{}
	add := func(d string) {
		d = strings.TrimSpace(d)
		if strings.HasPrefix(d, "~") && home != "" {
			d = home + d[1:]
		}
		if d != "" && !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	for _, p := range cfg.Paths {
		add(p)
	}
	for _, d := range lines[1:] {
		add(d)
	}
	return home, dirs, nil
}

func (r *RealRemote) host(name string) (model.HostConfig, bool) {
	for _, h := range r.hosts {
		if h.Name == name {
			return h, true
		}
	}
	return model.HostConfig{Name: name}, false
}

// LoginShell wraps a script to run in the user's login shell on the remote
// end of ssh: a plain `ssh host cmd` gets a shell that doesn't read the
// profile, which is often where ~/.local/bin joins the PATH.
func LoginShell(script string) string {
	return `exec "${SHELL:-/bin/sh}" -lc ` + Quote(script)
}

// Window is the command a kitty window opens host with: `kitten ssh`,
// forwarding kitty's remote control socket, in dir, the host told its own
// name and the window's id. It is interactive, for the ssh kitten to set the
// host's shell up; a command is typed in once its prompt shows.
func Window(host, dir string) []string {
	args := []string{"kitten", "ssh",
		"--kitten", "forward_remote_control=yes",
		"--kitten", "env=" + HostVar + "=" + host,
		"--kitten", "env=KITTY_WINDOW_ID=_kitty_copy_env_var_",
	}
	if dir != "" {
		args = append(args, "--kitten", "cwd="+dir)
	}
	return append(args, host)
}

// SessionName is the kitty session a directory on another machine opens
// as: its name, at the host's.
func SessionName(host, dir string) string {
	return path.Base(strings.TrimRight(dir, "/")) + "@" + host
}

// RemoteDir is a directory as the host's shell takes it: ~ is $HOME there.
func RemoteDir(dir string) string {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		return "$HOME" + dir[1:]
	}
	return dir
}

// ShellDir is a directory quoted for the host's shell, ~ expanding there.
func ShellDir(dir string) string {
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		return `"$HOME"` + Quote(dir[1:])
	}
	return Quote(dir)
}

// Quote quotes s for a POSIX shell.
func Quote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:@%+,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// Join quotes each argument and joins them into a command line.
func Join(argv ...string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = Quote(a)
	}
	return strings.Join(quoted, " ")
}
