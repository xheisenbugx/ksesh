package lister

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/xheisenbugx/ksesh/model"
)

// HostSessionName is how a directory on a [[host]] is listed: host:path,
// its home shortened to ~, which `ksesh connect` takes back.
func HostSessionName(host, home, dir string) string {
	if home != "" && (dir == home || strings.HasPrefix(dir, home+"/")) {
		dir = "~" + strings.TrimPrefix(dir, home)
	}
	return host + ":" + dir
}

// listHosts lists the directories of every [[host]], asking them all at
// once. A host that can't be reached is left out, not an error: the others
// are still worth listing.
func listHosts(l *RealLister) (model.SeshSessions, error) {
	out := model.SeshSessions{Directory: model.SeshSessionMap{}}
	if l.remote == nil {
		return out, nil
	}
	hosts := l.remote.Hosts()
	results := make([][]model.SeshSession, len(hosts))
	var wg sync.WaitGroup
	for i, h := range hosts {
		wg.Go(func() {
			home, dirs, err := l.remote.Dirs(h.Name)
			if err != nil {
				slog.Debug("lister: host unreachable", "host", h.Name, "error", err)
				return
			}
			for _, dir := range dirs {
				results[i] = append(results[i], model.SeshSession{
					Src:  "host",
					Name: HostSessionName(h.Name, home, dir),
					Path: dir,
					Host: h.Name,
				})
			}
		})
	}
	wg.Wait()
	for _, sessions := range results {
		for _, s := range sessions {
			key := "host:" + s.Name
			if _, dup := out.Directory[key]; dup {
				continue
			}
			out.Directory[key] = s
			out.OrderedIndex = append(out.OrderedIndex, key)
		}
	}
	return out, nil
}
