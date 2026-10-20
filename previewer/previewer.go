package previewer

import (
	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/icon"
	"github.com/xheisenbugx/ksesh/kitty"
	"github.com/xheisenbugx/ksesh/lister"
	"github.com/xheisenbugx/ksesh/ls"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/shell"
)

type Previewer interface {
	// Previews a session or directory
	Preview(name string) (string, error)
}

type RealPreviewer struct {
	icon       icon.Icon
	strategies []PreviewStrategy
}

func NewPreviewer(
	lister lister.Lister,
	kitty kitty.Kitty,
	icon icon.Icon,
	dir dir.Dir,
	home home.Home,
	ls ls.Ls,
	config model.Config,
	shell shell.Shell,
) Previewer {
	strategies := []PreviewStrategy{
		NewKittyStrategy(lister, kitty),
		NewConfigStrategy(lister, shell),
		NewDefaultConfigStrategy(lister, config, ls),
		NewDirectoryStrategy(home, dir, ls),
	}

	return &RealPreviewer{
		icon:       icon,
		strategies: strategies,
	}
}

func (p *RealPreviewer) Preview(name string) (string, error) {
	trimmedName := p.icon.RemoveIcon(name)

	for _, strategy := range p.strategies {
		output, err := strategy.Execute(trimmedName)
		if err != nil {
			return "", err
		}
		if output != "" {
			return output, nil
		}
	}
	return "", nil
}
