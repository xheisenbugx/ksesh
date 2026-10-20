package connector

import (
	"errors"
	"fmt"

	"github.com/xheisenbugx/ksesh/model"
)

// targetSession resolves the session a window or pane connect should land in.
// An empty name means the attached session, which is what makes `sesh window
// connect notes` work with no flags from inside kitty.
func (c *RealConnector) targetSession(name string) (model.SeshSession, error) {
	if name != "" {
		return c.ensureSession(name)
	}
	session, exists := c.lister.GetAttachedKittySession()
	if !exists {
		return model.SeshSession{}, errors.New("not inside a kitty session, use --target to specify one")
	}
	return session, nil
}

// ensureSession resolves a name to a running kitty session, starting it when it
// isn't running yet. Unlike Connect it never moves the client, so a background
// window or pane connect can target a session that doesn't exist yet without
// yanking the user out of the one they're in.
func (c *RealConnector) ensureSession(name string) (model.SeshSession, error) {
	connection, err := c.Resolve(name)
	if err != nil {
		return model.SeshSession{}, err
	}
	if !connection.Found {
		return model.SeshSession{}, fmt.Errorf("no session found for '%s'", name)
	}
	if connection.Session.Src == "kitty-window" {
		return model.SeshSession{}, fmt.Errorf("'%s' is a kitty window, not a session", name)
	}
	if connection.AddToZoxide {
		c.zoxide.Add(connection.Session.Path)
	}
	if !connection.New {
		return connection.Session, nil
	}
	if connection.Session.SessionFile != "" {
		if err := c.startSessionFile(connection.Session); err != nil {
			return model.SeshSession{}, err
		}
		return connection.Session, nil
	}
	if err := c.createSession(connection.Session, ""); err != nil {
		return model.SeshSession{}, err
	}
	return connection.Session, nil
}

// focusSession brings kitty to a session, and kitty to the front when sesh
// runs outside it.
func (c *RealConnector) focusSession(sessionName string, _ bool) (string, error) {
	msg, err := c.kitty.SwitchOrAttach(sessionName, model.ConnectOpts{})
	// Worth bringing kitty forward either way: on failure the user at least
	// lands where they can see what happened.
	c.activateTerminal()
	return msg, err
}
