package connector

import (
	"cmp"
	"fmt"

	"github.com/xheisenbugx/ksesh/model"
)

func kittyStrategy(c *RealConnector, name string) (model.Connection, error) {
	session, exists := c.lister.FindKittySession(name)
	if !exists {
		return model.Connection{Found: false}, nil
	}
	return model.Connection{
		Found:       true,
		Session:     session,
		New:         false,
		AddToZoxide: true,
	}, nil
}

func connectToKitty(c *RealConnector, connection model.Connection, opts model.ConnectOpts) (string, error) {
	if connection.New {
		if connection.Session.SessionFile != "" {
			return connectToSessionFile(c, connection, opts)
		}
		if err := c.createSession(connection.Session, opts.Command); err != nil {
			return "", err
		}
	}

	return c.focusSession(connection.Session.Name, opts.Switch)
}

// createSession starts a session and runs its startup, without focusing it.
// command, when set, replaces the configured startup.
func (c *RealConnector) createSession(session model.SeshSession, command string) error {
	standIn, err := c.ensureKitty(session.Path)
	if err != nil {
		return err
	}
	// Everything below assumes the session now exists, so a failure here
	// makes the rest meaningless — and it is how "kitten isn't on the PATH"
	// or "remote control is off" first shows up.
	target, err := c.kitty.NewSession(session.Name, session.Path)
	if err != nil {
		return fmt.Errorf("failed to create kitty session %q: %w", session.Name, err)
	}
	c.closeStandIn(standIn)
	if command != "" {
		c.kitty.SendKeys(target, command)
	} else {
		c.startup.Exec(session, target)
	}
	return nil
}

// ensureKitty makes sure there is a kitty to talk to. Outside kitty, with no
// instance reachable over a socket, it starts one, the way `sesh connect`
// outside tmux starts the tmux server. It returns the window the new kitty
// opened with, for closeStandIn to close once the session has a window of its
// own, or 0.
func (c *RealConnector) ensureKitty(startDir string) (int, error) {
	if c.kitty.Reachable() {
		return 0, nil
	}
	standIn, err := c.kitty.StartKitty(startDir)
	if err != nil {
		return 0, fmt.Errorf("couldn't reach kitty, and starting it failed: %w", err)
	}
	return standIn, nil
}

func (c *RealConnector) closeStandIn(windowID int) {
	if windowID != 0 {
		c.kitty.CloseWindow(windowID)
	}
}

// activateTerminal brings kitty to the front when sesh runs outside it, e.g.
// from a GUI launcher: switching sessions there changes nothing visible
// otherwise.
func (c *RealConnector) activateTerminal() {
	if !c.kitty.IsAttached() {
		c.focuser.Activate(cmp.Or(c.config.Terminal, "kitty"))
	}
}
