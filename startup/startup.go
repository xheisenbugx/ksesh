package startup

import (
	"fmt"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/replacer"
)

type Startup interface {
	Exec(session model.SeshSession, target string) (string, error)
}

type RealStartup struct {
	lister   lister.Lister
	kitty    kitty.Kitty
	config   model.Config
	home     home.Home
	replacer replacer.Replacer
}

func NewStartup(
	config model.Config, lister lister.Lister, kitty kitty.Kitty, home home.Home, replacer replacer.Replacer,
) Startup {
	return &RealStartup{lister, kitty, config, home, replacer}
}

// Exec runs a new session's startup: its configured windows become tabs, each
// running its startup_script, and the startup command is typed into the
// session's first window. target is that window, as NewSession returned it;
// empty means the session itself.
func (s *RealStartup) Exec(session model.SeshSession, target string) (string, error) {
	strategies := []func(*RealStartup, model.SeshSession) (string, error){
		configStrategy,
		configWildcardStartupStrategy,
		defaultConfigStrategy,
	}
	if target == "" {
		target = session.Name
	}

	windows := make(model.SeshWindowMap)
	for _, window := range s.config.WindowConfigs {
		key := lister.ConfigKey(window.Name)
		var path string = ""
		var err error = nil
		if window.Path != "" {
			path, err = s.home.ExpandPath(window.Path)
			if err != nil {
				return "", fmt.Errorf("couldn't expand home: %q", err)
			}
		}

		windows[key] = model.WindowConfig{
			Name:          window.Name,
			Path:          path,
			StartupScript: window.StartupScript,
		}
	}

	for _, window := range session.WindowNames {
		windowConfig, ok := windows[lister.ConfigKey(window)]
		if !ok {
			return "", fmt.Errorf("window %s is not defined in config", window)
		}
		if windowConfig.Path == "" {
			path, err := s.home.ExpandPath(session.Path)
			if err != nil {
				return "", fmt.Errorf("couldn't expand home: %q", err)
			}
			windowConfig.Path = path
		}

		// The tab is created in the background: the session opens on its
		// first tab, as it does in tmux.
		tabTarget, err := s.kitty.NewWindowInSession(model.KittyTabOpts{
			Name:          windowConfig.Name,
			StartDir:      windowConfig.Path,
			TargetSession: session.Name,
			Background:    true,
		})
		if err != nil {
			return tabTarget, err
		}
		if windowConfig.StartupScript != "" {
			if ret, err := s.kitty.SendKeys(tabTarget, windowConfig.StartupScript); err != nil {
				return ret, err
			}
		}
	}

	for _, strategy := range strategies {
		if command, err := strategy(s, session); err != nil {
			return "", fmt.Errorf("failed to determine startup command: %w", err)
		} else if command != "" {
			s.kitty.SendKeys(target, command)
			return fmt.Sprintf("executing startup command: %s", command), nil
		}
	}

	return "", nil // no command to run
}
