package connector

import (
	"fmt"

	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/model"
)

func sessionFileStrategy(c *RealConnector, name string) (model.Connection, error) {
	session, exists := c.lister.FindSessionFileConfig(name)
	if !exists {
		return model.Connection{Found: false}, nil
	}

	return model.Connection{
		Found:       true,
		Session:     session,
		New:         true,
		AddToZoxide: false,
	}, nil
}

// connectToSessionFile loads a kitty session file with goto_session, which
// switches to its session when it is already running.
func connectToSessionFile(c *RealConnector, connection model.Connection, _ model.ConnectOpts) (string, error) {
	if err := c.startSessionFile(connection.Session); err != nil {
		return "", err
	}
	c.activateTerminal()
	return fmt.Sprintf("switching to kitty session: %s", connection.Session.Name), nil
}

func (c *RealConnector) startSessionFile(session model.SeshSession) error {
	standIn, err := c.ensureKitty("")
	if err != nil {
		return err
	}
	native, err := c.kitty.GotoSessionFile(session.SessionFile)
	if err != nil {
		return err
	}
	c.closeStandIn(standIn)
	// Kitty names the session after the file. A [[session]] entry with a
	// session_file has a name of its own, which the session takes so sesh
	// finds it again.
	if native != session.Name && native == kitty.SessionFileName(session.SessionFile) {
		if err := c.kitty.SetSessionName(native, session.Name); err != nil {
			return fmt.Errorf("failed to name kitty session %q: %w", session.Name, err)
		}
	}
	return nil
}
