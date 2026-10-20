package previewer

import (
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
)

type KittyPreviewStrategy struct {
	lister lister.Lister
	kitty  kitty.Kitty
}

func NewKittyStrategy(lister lister.Lister, kitty kitty.Kitty) *KittyPreviewStrategy {
	return &KittyPreviewStrategy{lister: lister, kitty: kitty}
}

func (s *KittyPreviewStrategy) Execute(name string) (string, error) {
	session, sessionExists := s.lister.FindKittySession(name)

	if sessionExists {
		output, err := s.kitty.CapturePane(session.Name)
		if err != nil {
			return "", err
		}

		return output, nil
	}

	return "", nil
}
