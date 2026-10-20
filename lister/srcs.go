package lister

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/xheisenbugx/ksesh/model"
)

// returns a sorted list of sources based on the provided sort order.
func sortSources(sources []string, sortOrder model.SortOrder) []string {
	return slices.Concat(groupSources(sources, sortOrder)...)
}

// groupSources orders the active sources by sort_order and reports which of
// them share a block. A nested sort_order entry puts its sources in one group,
// whose sessions List merges and orders by score; every other source is a group
// of one, listed on its own exactly as it always has been.
//
// Sources sort_order doesn't mention keep their relative order and trail the
// ones it does, each as its own group.
func groupSources(sources []string, sortOrder model.SortOrder) [][]string {
	groups := sortOrder.SortGroups()
	if len(groups) == 0 {
		return singletonGroups(sources)
	}
	// A source named more than once takes the last position it was given, so
	// the group it ends up in is the last one that claimed it.
	m := make(map[string]srcPos)
	for i, group := range groups {
		for j, src := range group {
			m[canonicalSource(src)] = srcPos{group: i, within: j}
		}
	}
	unlisted := srcPos{group: math.MaxInt, within: math.MaxInt}
	getOrder := func(s string) srcPos {
		if pos, exists := m[strings.ToLower(s)]; exists {
			return pos
		}
		return unlisted
	}
	result := slices.Clone(sources)
	slices.SortStableFunc(result, func(a, b string) int {
		pa, pb := getOrder(a), getOrder(b)
		if pa.group != pb.group {
			return cmp.Compare(pa.group, pb.group)
		}
		return cmp.Compare(pa.within, pb.within)
	})

	grouped := make([][]string, 0, len(result))
	prev := math.MaxInt
	for _, src := range result {
		group := getOrder(src).group
		// MaxInt is "unlisted" rather than a shared position, so those sources
		// never merge with each other.
		if group != math.MaxInt && group == prev && len(grouped) > 0 {
			last := len(grouped) - 1
			grouped[last] = append(grouped[last], src)
			continue
		}
		grouped = append(grouped, []string{src})
		prev = group
	}
	return grouped
}

// legacySources maps sesh's source names to their kitty counterparts, so a
// sort_order written for sesh keeps working.
var legacySources = map[string]string{
	"tmux":       "kitty",
	"tmuxinator": "sessionfile",
	"tmux-pane":  "kitty-window",
}

func canonicalSource(src string) string {
	src = strings.ToLower(src)
	if kitty, ok := legacySources[src]; ok {
		return kitty
	}
	return src
}

// srcPos is where sort_order puts a source: which group, and where inside it.
// The position inside the group is what breaks ties between sessions a merged
// group scores the same, so a group reads in the order it was written.
type srcPos struct {
	group  int
	within int
}

func singletonGroups(sources []string) [][]string {
	groups := make([][]string, len(sources))
	for i, src := range sources {
		groups[i] = []string{src}
	}
	return groups
}

func srcs(opts ListOptions) []string {
	count := 0
	if opts.Kitty {
		count++
	}
	if opts.Config {
		count++
	}
	if opts.SessionFile {
		count++
	}
	if opts.Zoxide {
		count++
	}
	if opts.Panes {
		count++
	}
	if count == 0 {
		return []string{"kitty", "config", "sessionfile", "zoxide"}
	}
	srcs := make([]string, 0, count)
	if opts.Kitty {
		srcs = append(srcs, "kitty")
	}
	if opts.Config {
		srcs = append(srcs, "config")
	}
	if opts.SessionFile {
		srcs = append(srcs, "sessionfile")
	}
	if opts.Zoxide {
		srcs = append(srcs, "zoxide")
	}
	if opts.Panes {
		srcs = append(srcs, "kitty-window")
	}
	return srcs
}
