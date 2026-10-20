# ksesh — sesh for kitty

`ksesh` is a port of [sesh](https://github.com/joshmedeski/sesh), the smart
terminal session manager, that uses **kitty's native sessions** instead of tmux.
It keeps sesh's features (zoxide integration, the picker TUI, config sessions,
startup commands, windows, wildcards, aliases, git worktrees, previews, caching),
but every session it creates or switches to is a real kitty session, so kitty's
own `goto_session`, `tab_bar_filter session:…` and `{session_name}` work with
them too.

Requires kitty **0.43+** (sessions were introduced in 0.43) and `zoxide`.

## How sesh concepts map onto kitty

| sesh / tmux              | ksesh / kitty                                                                 |
| ------------------------ | ----------------------------------------------------------------------------- |
| tmux session             | kitty session: the windows kitty tags with the same `session_name`            |
| tmux window              | kitty tab in that session (`ksesh tab …`, alias `ksesh window …`)              |
| tmux pane                | kitty window (`ksesh list -p`)                                                 |
| `tmux new-session`       | `kitten @ launch --type=tab --add-to-session NAME --cwd DIR`                   |
| `tmux switch-client`     | `goto_session` (so kitty's session history and `goto_session -1` follow)       |
| `send-keys` startup cmd  | `kitten @ send-text` into the session's first window                           |
| `capture-pane` preview   | `kitten @ get-text --ansi`                                                    |
| `kill-session`           | closes every window of the session                                             |
| tmuxinator projects      | kitty session files (`*.kitty-session`) in `session_file_dirs`                 |
| attached session         | the session of the kitty window ksesh runs in (or the focused one, outside)   |

## Install

```sh
git clone <this repo> && cd kitty-sesh
go build -o ~/.local/bin/ksesh .
```

## kitty setup

Add this to `kitty.conf` (restart kitty afterwards — `listen_on` can't be
reloaded):

```conf
# ksesh drives kitty over remote control.
allow_remote_control socket-only
# A socket lets ksesh reach kitty from anywhere (GUI launchers, Raycast…) and
# keeps remote control off the tty while the picker is up.
listen_on unix:/tmp/kitty

# tmux-like: each session has its own tabs; the tab bar shows only the
# active session's tabs, and cmd+1..9 count only those.
tab_bar_filter session:.
# Works around a kitty bug with that filter: without it, closing a tab can jump
# to a tab of the session you were in before. Copy contrib/ksesh_watcher.py
# next to kitty.conf.
watcher ksesh_watcher.py
tab_title_template "{index} {title}"
active_tab_title_template "{index} {title}{' · ' + session_name if session_name else ''}"

# Start kitty inside a session ("main") so no tab is left outside one. The
# file can be as small as "cd ~" and "launch".
startup_session sessions/main.kitty-session

# New tabs and windows join the session you're in.
map kitty_mod+t     new_tab_with_cwd
map kitty_mod+enter new_window_with_cwd
map super+n           new_os_window_with_cwd

# Session picker in an overlay, and jump back to the previous session.
map super+p launch --type=overlay --cwd=current ksesh picker -i
map super+l goto_session -1
```

Without `listen_on`, ksesh still works from inside kitty with
`allow_remote_control yes`: it then talks to kitty through the terminal, and the
picker snapshots kitty before it draws (previews included) and defers `ctrl+x`
kills until it closes, since a TUI and `kitten @` can't share the tty.

If kitty can't find `ksesh` from a key binding, use its full path
(`~/.local/bin/ksesh`).

## Usage

Everything from sesh works the same way; only the multiplexer changed.

```sh
ksesh list                 # sessions from kitty, config, session files and zoxide
ksesh list -t              # only running kitty sessions (-c config, -T session files, -z zoxide)
ksesh list -i --json       # icons / JSON, as in sesh
ksesh connect <name|dir>   # switch to it, creating it when needed
ksesh picker -i --preview  # the sesh picker TUI
ksesh last                 # the session you were in before this one
ksesh preview <name>       # live screen of a session, or a directory listing
ksesh root                 # root of the current session
ksesh kill <name>          # kill a session (the attached one without a name)
ksesh mkdir <path>         # mkdir + connect
ksesh clone <repo>         # git clone + connect
ksesh rename --to <name>   # rename the current session (or: ksesh rename <session> --to …)
ksesh rename --enrich      # add the GitHub issue title, as in sesh
ksesh tab list             # tabs of the current session (-t <session> for another)
ksesh tab connect logs     # select the "logs" tab, creating it if missing
ksesh tab connect ~/c/api -c 'npm run dev'
ksesh list -p              # kitty windows of the current session
ksesh worktree connect 42  # git worktree for issue/PR #42 as a session
```

With fzf instead of the built-in picker:

```sh
ksesh connect "$(ksesh list -i | fzf --ansi --no-sort)"
```

[`contrib/ksesh-fzf`](contrib/ksesh-fzf) is the kitty version of sesh's
fzf-tmux popup binding: `^a` all, `^t` kitty sessions, `^g` configs, `^s`
session files, `^x` zoxide, `^f` find directories (needs `fd`), `^d` kill the
highlighted session, with `ksesh preview` on the right. In a tab split into
several windows it zooms the tab (the `stack` layout) while it is open, so the
picker gets the whole tab. Copy it next to
`kitty.conf` and map it to an overlay (it needs `listen_on`):

```conf
map super+t launch --type=overlay --cwd=current ~/.config/kitty/ksesh-fzf
```

`ksesh kill [name]` closes every window of a session (the attached one without
a name); like `connect`, it accepts a line from `ksesh list -i` as is.

Running ksesh **outside kitty**: it reaches kitty through `$KITTY_LISTEN_ON` or
the `kitty_socket` option and brings kitty to the front after switching. When no
kitty can be reached at all, `ksesh connect` starts one (the equivalent of sesh
starting the tmux server).

## Configuration

ksesh reads `~/.config/ksesh/ksesh.toml`, and falls back to
`~/.config/sesh/sesh.toml`, so an existing sesh config works as is. `-C` picks
another file. Schema: [`ksesh.schema.json`](ksesh.schema.json).

```toml
#:schema ./ksesh.schema.json

# kitty-specific options (all optional)
kitty_socket = "unix:/tmp/kitty"   # reach kitty from outside it; kitty's -<pid> suffix is found
new_os_window = false              # true: each new session gets its own OS window
session_file_dirs = ["~/.config/kitty/sessions"]  # listed as the "sessionfile" source
kitten_command = "kitten"
kitty_command = "kitty"            # used only to start kitty when none is running
terminal = "kitty"                 # app brought forward when switching from outside kitty

[default_session]
startup_command = "nvim"

[[session]]
name = "dotfiles"
path = "~/dotfiles"
startup_command = "nvim"
alias = "d"
windows = ["agent", "server"]      # extra tabs, from [[window]] below

[[session]]
name = "work"
path = "~/c/work"
session_file = "~/c/work/work.kitty-session"  # start from a kitty session file instead

[[window]]
name = "agent"
startup_script = "codex"

[[window]]
name = "server"

[tui]
show_icons = true
preview = true
window_name_format = "{index}:{title}"  # {title} {index} {id} {cwd} {cwd_name} {active} {active_title}
```

All other sesh options behave as documented in the
[sesh README](https://github.com/joshmedeski/sesh#configuration): `blacklist`,
`sort_order` (sources: `kitty`, `config`, `sessionfile`, `zoxide`; sesh's
`tmux`/`tmuxinator` names are accepted), `dir_length`, `name_substitution`,
`[[wildcard]]`, `[[worktree]]`, `cache`, `[frecency]`, `[tui]`, aliases and
`import`.

### Kitty session files

Session files in `session_file_dirs` are listed (source `sessionfile`, flag
`-T`) and connecting to one runs `goto_session` on it, so kitty builds the
layout. Kitty names the session after the file, e.g. `api.kitty-session` →
`api`. Create one from a live setup with kitty's
`save_as_session --use-foreground-process`.

### Differences from sesh

- `ksesh tab` replaces `sesh window` (`window`/`w` still work as aliases); it
  prints `tab:<id>` / `window:<id>` targets instead of `session:index`.
- `connect -T` (tmuxinator) is gone; use session files. `tmuxp` and
  `tmuxinator` config keys are replaced by `session_file`.
- Kitty can't rename a session, so `ksesh rename` stores the new name in a
  `ksesh_name` user variable on the session's windows; kitty keeps its own name
  for `goto_session` and `tab_bar_filter`.
- `--switch` is accepted for compatibility; switching works the same from
  inside and outside kitty.
- `rename --to` is new; sesh only implements `--enrich`.

## Credits

ksesh is a fork of [sesh](https://github.com/joshmedeski/sesh) by Josh Medeski,
MIT licensed.
