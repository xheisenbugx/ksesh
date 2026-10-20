package lister

import (
	"fmt"
	"os"

	"github.com/xheisenbugx/ksesh/model"
)

func kittyWindowKey(windowID int) string {
	return fmt.Sprintf("kitty-window:%d", windowID)
}

func kittyWindowDisplayName(window *model.KittyWindow) string {
	hostname, _ := os.Hostname()
	if window.Title != "" && window.Title != hostname {
		return window.Title
	}
	return window.Command
}

func listKittyWindows(l *RealLister) (model.SeshSessions, error) {
	kittyWindows, err := l.kitty.ListKittyWindows()
	if err != nil {
		return model.SeshSessions{}, fmt.Errorf("couldn't list kitty windows: %q", err)
	}

	// Count raw names to detect duplicates needing .0, .1 suffixes
	type windowEntry struct {
		window  *model.KittyWindow
		rawName string
	}
	entries := make([]windowEntry, len(kittyWindows))
	nameCounts := make(map[string]int)
	for i, window := range kittyWindows {
		rawName := fmt.Sprintf("%s/%s", window.TabTitle, kittyWindowDisplayName(window))
		entries[i] = windowEntry{window: window, rawName: rawName}
		nameCounts[rawName]++
	}

	directory := make(map[string]model.SeshSession)
	orderedIndex := []string{}
	nameIndexes := make(map[string]int)

	for _, entry := range entries {
		name := entry.rawName
		if nameCounts[entry.rawName] > 1 {
			idx := nameIndexes[entry.rawName]
			name = fmt.Sprintf("%s.%d", entry.rawName, idx)
			nameIndexes[entry.rawName] = idx + 1
		}

		key := kittyWindowKey(entry.window.ID)
		orderedIndex = append(orderedIndex, key)
		directory[key] = model.SeshSession{
			Src:  "kitty-window",
			Name: name,
			Path: entry.window.Path,
		}
	}

	return model.SeshSessions{
		Directory:    directory,
		OrderedIndex: orderedIndex,
	}, nil
}

func (l *RealLister) ListKittyWindows() (model.SeshSessions, error) {
	return listKittyWindows(l)
}
