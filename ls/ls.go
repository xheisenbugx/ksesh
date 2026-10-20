package ls

import (
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/shell"
)

type Ls interface {
	ListDirectory(name string) (string, error)
}

type RealLs struct {
	config model.Config
	shell  shell.Shell
}

func NewLs(config model.Config, shell shell.Shell) Ls {
	return &RealLs{config, shell}
}

func (g *RealLs) ListDirectory(path string) (string, error) {
	command := g.config.DefaultSessionConfig.PreviewCommand
	if command == "" {
		command = "ls {}"
	}

	cmdOutput, err := g.shell.ShellCmd(command, map[string]string{"{}": path})
	if err != nil {
		return "", err
	}
	return cmdOutput, nil
}
