// Package contrib holds ksesh's kitty-side files, which `ksesh setup` copies
// next to kitty.conf, and the template of the kitty config it writes there.
package contrib

import "embed"

// Files are the files `ksesh setup` installs, by name.
//
//go:embed ksesh_watcher.py ksesh_agents.py ksesh_snapshot.py ksesh_reload.py tab_bar.py ksesh-fzf ksesh-agents-fzf ksesh.conf.tmpl
var Files embed.FS
