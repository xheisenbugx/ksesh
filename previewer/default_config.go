package previewer

import (
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/ls"
	"github.com/xheisenbugx/ksesh/model"
)

type DefaultConfigPreviewStrategy struct {
	lister lister.Lister
	config model.Config
	ls     ls.Ls
}

func NewDefaultConfigStrategy(lister lister.Lister, config model.Config, ls ls.Ls) *DefaultConfigPreviewStrategy {
	return &DefaultConfigPreviewStrategy{lister: lister, config: config, ls: ls}
}

func (s *DefaultConfigPreviewStrategy) Execute(name string) (string, error) {
	session, configExists := s.lister.FindConfigSession(name)
	if !configExists {
		return "", nil
	}

	out, err := s.ls.ListDirectory(session.Path)
	if err != nil {
		return "", err
	}
	return out, nil
}
