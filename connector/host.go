package connector

import (
	"fmt"
	"strings"
	"time"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/remote"
)

// remoteShellWait is how long a new window on another machine is given to
// show its shell's prompt before a command is typed into it.
const remoteShellWait = 20 * time.Second

// hostStrategy connects to a directory on a [[host]], named host:path as
// `ksesh list --hosts` lists it: a kitty session on that machine, over
// `kitten ssh`, named dir@host. One already open is gone to.
func hostStrategy(c *RealConnector, name string) (model.Connection, error) {
	host, dir, ok := strings.Cut(name, ":")
	if !ok || dir == "" || !c.isHost(host) {
		return model.Connection{Found: false}, nil
	}
	session := model.SeshSession{Src: "host", Name: remote.SessionName(host, dir), Path: dir, Host: host}
	if existing, ok := c.lister.FindKittySession(session.Name); ok {
		return model.Connection{Found: true, Session: existing}, nil
	}
	return model.Connection{Found: true, Session: session, New: true}, nil
}

func (c *RealConnector) isHost(name string) bool {
	for _, h := range c.hosts {
		if h.Name == name {
			return true
		}
	}
	return false
}

// connectToHost opens a directory on another machine as a kitty session:
// a window running `kitten ssh` there, forwarding kitty's remote control so
// the agents started in it report here. Its windows carry the host and
// directory, which the sidebar shows threads started in them on.
func connectToHost(c *RealConnector, connection model.Connection, opts model.ConnectOpts) (string, error) {
	s := connection.Session
	if connection.New {
		dir := remote.RemoteDir(s.Path)
		command := remote.Join(remote.Window(s.Host, dir)...)
		vars := map[string]string{kitty.HostVar: s.Host, kitty.CwdVar: s.Path}
		target, err := c.kitty.NewSessionWith(s.Name, "", command, vars)
		if err != nil {
			return "", fmt.Errorf("failed to open %s on %s: %w", s.Path, s.Host, err)
		}
		if opts.Command != "" {
			if err := c.kitty.WaitForPrompt(target, remoteShellWait); err == nil {
				c.kitty.SendKeys(target, opts.Command)
			}
		}
	}
	return c.focusSession(s.Name, opts.Switch)
}
