package previewer

import (
	"strings"

	"github.com/xheisenbugx/ksesh/remote"
)

// HostStrategy previews a directory on a [[host]], named host:path: its
// listing there, over ssh.
type HostStrategy struct {
	remote remote.Remote
}

func NewHostStrategy(r remote.Remote) *HostStrategy { return &HostStrategy{remote: r} }

func (s *HostStrategy) Execute(name string) (string, error) {
	host, dir, ok := strings.Cut(name, ":")
	if !ok || dir == "" {
		return "", nil
	}
	known := false
	for _, h := range s.remote.Hosts() {
		known = known || h.Name == host
	}
	if !known {
		return "", nil
	}
	out, err := s.remote.Run(host, "", "sh", "-c", "cd "+remote.ShellDir(dir)+" && ls -la")
	if err != nil {
		return "couldn't reach " + host + ": " + err.Error(), nil
	}
	return out, nil
}
