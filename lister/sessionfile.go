package lister

import (
	"fmt"

	"github.com/xheisenbugx/ksesh/model"
)

func sessionfileKey(name string) string {
	return fmt.Sprintf("sessionfile:%s", name)
}

func listSessionFile(l *RealLister) (model.SeshSessions, error) {
	sessionfileResults, err := l.sessionfile.List()
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("couldn't list kitty session files: %q", err)
	}

	numSessionFileResults := len(sessionfileResults)
	orderedIndex := make([]string, numSessionFileResults)
	directory := make(model.SeshSessionMap)

	for i, session := range sessionfileResults {
		key := sessionfileKey(session.Name)
		orderedIndex[i] = key
		directory[key] = model.SeshSession{
			Src:         "sessionfile",
			Name:        session.Name,
			Path:        session.Path,
			SessionFile: session.Path,
		}
	}
	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
}

func (l *RealLister) FindSessionFileConfig(name string) (model.SeshSession, bool) {
	sessions, _ := listSessionFile(l)
	key := sessionfileKey(name)
	if session, exists := sessions.Directory[key]; exists {
		return session, exists
	} else {
		return model.SeshSession{}, false
	}
}
