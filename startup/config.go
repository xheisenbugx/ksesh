package startup

import "github.com/xheisenbugx/ksesh/model"

func configStrategy(s *RealStartup, session model.SeshSession) (string, error) {
	config, exists := s.lister.FindConfigSession(session.Name)

	if exists && config.StartupCommand != "" {
		replacements := map[string]string{
			"{}": session.Path,
		}

		return s.replacer.Replace(config.StartupCommand, replacements), nil
	}
	return "", nil
}
