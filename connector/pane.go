package connector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xheisenbugx/ksesh/model"
)

func isKittyWindowFormat(name string) bool {
	return strings.Contains(name, "/") && !strings.HasPrefix(name, "/")
}

func kittyWindowStrategy(c *RealConnector, name string) (model.Connection, error) {
	if !c.kitty.IsAttached() || !isKittyWindowFormat(name) {
		return model.Connection{Found: false}, nil
	}

	sessions, err := c.lister.ListKittyWindows()
	if err != nil {
		return model.Connection{Found: false}, nil
	}

	for _, key := range sessions.OrderedIndex {
		session := sessions.Directory[key]
		if session.Name == name {
			return model.Connection{
				Found:       true,
				Session:     session,
				New:         false,
				AddToZoxide: false,
			}, nil
		}
	}

	return model.Connection{Found: false}, nil
}

func connectToKittyWindow(c *RealConnector, connection model.Connection, _ model.ConnectOpts) (string, error) {
	sessions, err := c.lister.ListKittyWindows()
	if err != nil {
		return "", err
	}

	for _, key := range sessions.OrderedIndex {
		if sessions.Directory[key].Name != connection.Session.Name {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(key, "kitty-window:"))
		if err != nil {
			return "", fmt.Errorf("unexpected kitty window key: %s", key)
		}
		return c.kitty.SelectPane(id)
	}

	return "", fmt.Errorf("window not found: %s", connection.Session.Name)
}
