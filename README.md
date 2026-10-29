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
ksesh wt new feat/login    # git worktree for a branch as a session (see Worktrees)
ksesh agent next           # jump to the agent that needs you (see Agents)
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

## Agents

ksesh tracks the coding agents running in kitty, so you can see at a glance
which one needs you and jump straight to it, without tmux.

Each agent reports its status on its own kitty window as user variables
(`ksesh_status` = `working`, `waiting`, `done`, `error` or `idle`, plus
`ksesh_msg`, `ksesh_prompt`, `ksesh_status_at` and `ksesh_turn_at`). For Claude
Code, `ksesh agent install` adds hooks to `~/.claude/settings.json` that run
`ksesh agent hook` on every event that changes it (prompt submitted → working,
permission prompt → waiting, stop → done, API failure → error), keeping your
other hooks. A permission prompt's message says what it would allow (`Allow
Bash: npm test`, `Allow Edit: src/main.go`), so you can answer it from the
picker without looking at the agent. Other agents can call `ksesh agent set waiting -m "…"` from their
own hooks; agents without any (codex, gemini, aider, …) are still listed, as
idle, by their program.

```sh
ksesh agent install          # Claude Code hooks (uninstall removes them)
ksesh agent list             # every agent, the ones that need you first
ksesh agent list -A          # only waiting, failed or done
ksesh agent next             # go to the one waiting longest; again for the next
ksesh agent last             # back to the agent you were with before
ksesh agent set done -m "…"  # report a status by hand or from another agent
ksesh agent send 42 --key enter --if-status waiting  # approve without switching
ksesh agent send 42 "use the existing helper"         # reply without switching
ksesh agent show 42          # status, branch, changes and screen of window 42
ksesh agent diff 42          # review it: branch vs base, commits, files, diff
```

`ksesh agent diff` reviews the agent's worktree the way its branch will be
reviewed: everything since it left its base (origin's default branch), so
commits, uncommitted changes and new files alike, with the files not committed
yet marked `●`. The diff is drawn with line numbers and tinted backgrounds, and
goes through [delta](https://github.com/dandavison/delta) instead when it is
installed.

[`contrib/ksesh-agents-fzf`](contrib/ksesh-agents-fzf) is a live picker over
them (it refreshes every 2 seconds), with the agent's screen, or its git diff,
on the right, from which most things you'd switch to an agent for can be done
in place. Its footer only shows the keys that do something for the selected
agent (approve and deny for a waiting one, interrupt for a working one, …):

| key      | does                                                    |
| -------- | ------------------------------------------------------- |
| `enter`  | go to the agent                                         |
| `ctrl-y` | approve: enter in a waiting agent's permission prompt   |
| `ctrl-x` | interrupt a working agent, deny a waiting one (escape)  |
| `ctrl-o` | reply: type a message and send it                       |
| `ctrl-s` | mark seen (back to idle)                                |
| `ctrl-g` | preview the review (`ksesh agent diff`) instead of the screen, and back; files with hundreds of changed lines are folded |
| `alt-d`  | the full review in `less`, nothing folded               |
| `ctrl-d` | scroll the preview half a page down (`ctrl-u` up)       |
| `alt-b`  | open the branch's pull request                          |
| `alt-w`  | close the agent's window (asks first)                   |

The status also shows wherever sessions do: `ksesh list -i`, `ksesh-fzf` and
the picker draw a session's most urgent agent status in place of its kitty
icon, and list sessions that need you first.

In kitty itself, [`contrib/ksesh_agents.py`](contrib/ksesh_agents.py) adds:

- a status icon on each tab with an agent;
- at the right end of the tab bar, an **attention strip** naming every *other*
  session with an agent waiting on you or failed. `tab_bar_filter session:.`
  hides other sessions' tabs, and the strip is how you still see them;
- a desktop notification when an agent starts waiting, fails or finishes, only
  if you aren't looking at its window. Clicking it focuses the window, and it
  goes away on its own once the agent moves on (say you approved it from the
  picker). A turn that finishes in under 30 seconds doesn't notify;
- "done" turns back to idle once you focus the agent's window, or right away
  if it finishes while you're looking at it.

Copy `contrib/ksesh_agents.py`, `contrib/ksesh_watcher.py`,
`contrib/tab_bar.py` and `contrib/ksesh_reload.py` next to `kitty.conf`
(merge `tab_bar.py` into yours if you have one), and add:

```conf
watcher ksesh_watcher.py
tab_title_template "{custom}"
tab_bar_style custom

# Next agent that needs you / back to the previous agent / agent picker.
map super+j       launch --type=background ksesh agent next
map super+shift+j launch --type=background ksesh agent last
map super+a       launch --type=overlay --cwd=current ~/.config/kitty/ksesh-agents-fzf
```

kitty reads `tab_bar.py` and watchers once and keeps them until it quits.
`kitten @ kitten ksesh_reload.py` loads new ones into the running kitty, so
your sessions don't have to go. `ksesh_agents.py` itself is re-read whenever it
changes. The hooks need `listen_on`: without a socket they do nothing rather
than talk to kitty over Claude's own terminal.

## Worktrees

Besides `ksesh worktree connect <issue>` (GitHub issues and PRs, below), ksesh
manages plain branch worktrees in any repo, workmux-style, as kitty sessions:

```sh
ksesh wt new feat/login                        # worktree + session for a branch
ksesh wt new feat/login -p "add a login page"  # …and start claude with a prompt
ksesh wt new fix/typo --agent codex -b origin/release
ksesh wt status                                # every worktree: agent, changes, ↑↓ vs base
ksesh wt rm                                    # remove this session's worktree, branch and session
ksesh wt rm --gone --merged                    # clean up the ones whose PR was merged
```

`wt new` checks out the branch when it exists locally or on `origin`, and
creates it from `--base` (default: `base_branch`, else origin's default
branch) otherwise. Worktrees go in the repo's `[[worktree]] worktree_dir`, or
next to the repo in `<repo>__worktrees/<branch>` without a block. A new
worktree gets the files listed in `.worktreeinclude` (the file Claude Code and
others use for this, as paths and globs) and in `copy` copied in from the main
worktree, and `symlink` linked. The session then runs `create_command`, then the
agent (`agent_command`, default `claude`):

```toml
[[worktree]]
path = "~/c/app"
worktree_dir = "../app__worktrees"
copy = [".env", "config/*.local"]
symlink = ["node_modules"]
create_command = "pnpm install"
agent_command = "claude"
```

`wt rm` refuses a worktree with uncommitted changes and keeps an unmerged
branch unless `--force`. A `--gone` branch (upstream deleted) counts as merged,
since a squash-merged PR never looks merged to git.

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
