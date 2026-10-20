// Package sessionfile finds kitty session files, the kitty counterpart of
// tmuxinator projects: each file describes a session kitty creates with
// goto_session, named after the file.
package sessionfile

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
)

// DefaultDir is scanned when session_file_dirs isn't set.
const DefaultDir = "~/.config/kitty/sessions"

// extensions are the ones kitty itself recognizes as session files.
var extensions = []string{".kitty-session", ".kitty_session", ".session"}

type SessionFile interface {
	List() ([]*model.SessionFileConfig, error)
}

type RealSessionFile struct {
	os   oswrap.Os
	home home.Home
	dirs []string
}

func NewSessionFile(os oswrap.Os, home home.Home, dirs []string) SessionFile {
	if len(dirs) == 0 {
		dirs = []string{DefaultDir}
	}
	return &RealSessionFile{os: os, home: home, dirs: dirs}
}

// List returns the session files in the configured directories, sorted by
// name. A name found in more than one directory is listed once, from the first
// directory that has it. Missing directories are skipped.
func (s *RealSessionFile) List() ([]*model.SessionFileConfig, error) {
	seen := map[string]bool{}
	configs := []*model.SessionFileConfig{}
	for _, dir := range s.dirs {
		expanded, err := s.home.ExpandPath(dir)
		if err != nil {
			continue
		}
		entries, err := s.os.ReadDir(expanded)
		if err != nil {
			continue
		}
		var found []*model.SessionFileConfig
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name, ok := stem(entry.Name())
			if !ok || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			found = append(found, &model.SessionFileConfig{
				Name: name,
				Path: filepath.Join(expanded, entry.Name()),
			})
		}
		sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
		configs = append(configs, found...)
	}
	return configs, nil
}

func stem(fileName string) (string, bool) {
	for _, ext := range extensions {
		if name, ok := strings.CutSuffix(fileName, ext); ok {
			return name, true
		}
	}
	return "", false
}
