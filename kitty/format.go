package kitty

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xheisenbugx/ksesh/model"
)

// FormatTab renders a tab for [tui] window_name_format. It understands:
//
//	{title}        the tab title
//	{index}        the tab's 1-based position in its session
//	{id}           kitty's tab id
//	{cwd}          the working directory of the tab's active window
//	{cwd_name}     the last element of {cwd}
//	{active}       "*" for the session's active tab, "" otherwise
//	{active_title} the title for the session's active tab, "" otherwise
//
// The tmux formats sesh configs commonly carry (#{window_name},
// #{window_index}, #{pane_current_path}) are accepted as aliases, so an
// existing sesh config keeps working.
func FormatTab(format string, tab model.KittyTab) string {
	if format == "" {
		format = model.DefaultWindowNameFormat
	}
	active, activeTitle := "", ""
	if tab.Active {
		active, activeTitle = "*", tab.Name
	}
	return strings.NewReplacer(
		"#{window_name}", tab.Name,
		"#{window_index}", strconv.Itoa(tab.Index),
		"#{pane_current_path}", tab.Path,
		"{title}", tab.Name,
		"{index}", strconv.Itoa(tab.Index),
		"{id}", strconv.Itoa(tab.ID),
		"{cwd_name}", filepath.Base(tab.Path),
		"{cwd}", tab.Path,
		"{active_title}", activeTitle,
		"{active}", active,
	).Replace(format)
}
