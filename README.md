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
`ksesh_msg`, `ksesh_prompt`, `ksesh_status_at` and `ksesh_turn_at`), set by
`ksesh agent hook` from the agent's own hooks, which `ksesh agent install` adds
for you, keeping your other settings and hooks (the file it changes is backed
up next to it, and `ksesh agent uninstall` takes them out again):

| agent       | install                        | working                                    | waiting                     | done               | error                |
| ----------- | ------------------------------ | ------------------------------------------ | --------------------------- | ------------------ | -------------------- |
| Claude Code | `ksesh agent install`          | prompt submitted, tool ran                 | permission prompt, question | stop               | API failure          |
| Codex       | `ksesh agent install --codex`  | prompt submitted, tool ran, model answered | approval request            | stop (or `notify`) | model request failed |
| Gemini CLI  | `ksesh agent install --gemini` | prompt submitted, tool started             | tool confirmation           | agent finished     | (no event for it)    |

`--all` installs all three. A permission prompt's message says what it would
allow (`Allow Bash: npm test`, `Allow shell: npm test`, `Allow edit:
src/main.go`, `Allow network: github.com`), so you can answer it from the
picker without looking at the agent.

- **Claude Code**: hooks in `~/.claude/settings.json` (or `--settings`).
- **Codex**: hooks in `~/.codex/hooks.json` (`$CODEX_HOME`, or
  `--codex-home`), or, when your `config.toml` already defines `[hooks]` of its
  own, in a marked block at its end, since Codex warns when one folder has both.
  Codex only runs a hook you trusted: after installing, open `/hooks` in Codex
  and trust ksesh's (once; reinstalling the same binary keeps them trusted).
  Your `notify` program is left alone: the hooks report everything it would.
  On a Codex too old for hooks, set `notify = ["ksesh", "agent", "hook",
  "--agent", "codex"]` yourself for "done".
- **Gemini CLI**: hooks in `~/.gemini/settings.json` (or `--gemini-settings`).
  ksesh can't edit a `settings.json` with comments in it.

Other agents can call `ksesh agent set waiting -m "…"` from their own hooks;
agents without any (aider, opencode, …) are still listed, as idle, by their
program.

```sh
ksesh agent install          # Claude Code hooks (uninstall removes them)
ksesh agent install --codex  # or --gemini, or --all
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
installed. `--pager` pages it in `less`.

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
`contrib/ksesh_snapshot.py`, `contrib/tab_bar.py` and `contrib/ksesh_reload.py`
next to `kitty.conf` (merge `tab_bar.py` into yours if you have one), and add:

```conf
watcher ksesh_watcher.py
tab_title_template "{custom}"
tab_bar_style custom

# Next agent that needs you / back to the previous agent / agent picker.
map super+j       launch --type=background --allow-remote-control ksesh agent next
map super+shift+j launch --type=background --allow-remote-control ksesh agent last
map super+a       launch --type=overlay --cwd=current ~/.config/kitty/ksesh-agents-fzf
```

kitty reads `tab_bar.py` and watchers once and keeps them until it quits.
`kitten @ kitten ksesh_reload.py` loads new ones into the running kitty, so
your sessions don't have to go. `ksesh_agents.py` and `ksesh_snapshot.py`
themselves are re-read whenever they change. The hooks need `listen_on`:
without a socket they do nothing rather than talk to kitty over the agent's own
terminal.

## Threads and the sidebar

With many agents going, a list sorted by urgency isn't enough: some you want
kept in view, some you want out of the way until later, some you are done
with. ksesh triages agents the way [T3 Code](https://github.com/pingdotgg/t3code)
triages its threads, and shows them in a **sidebar docked to the left of every
tab** (kitty 0.50's `launch --type=tab-dock`):

```
 Threads ╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱╱
 all sessions · 6              1   1   the scope, and what needs you

  PINNED ──────────────────────── 1
   Refactor auth                1h
    api · › split the session store
                                         Pinned  yours, in your order
  INBOX ───────────────────────── 2     Inbox   the latest activity first
▌  Fix the login redirect       2m ●   ▌ selected   ● the thread you're in
▌   api · Allow Bash: npm test
   Write the README             5m
    docs · Added a Threads section

 ────────────────────────────────────
 ▸ ⠹ Working                        2   agents running that don't need you
 ▸  Snoozed                        1   hidden until a time you picked
 ▸  Settled                        9   done with
 ────────────────────────────────────
 a approve · x deny · r reply · ? keys
```

The look borrows from [Crush](https://github.com/charmbracelet/crush): a
gradient masthead running out into stripes, section labels with a rule to
their count, and a status bar. Its colors are mixed from your terminal's own
background and foreground, so it suits light and dark themes alike; the
selected row is tinted with the accent and marked with a bar, the one under
the mouse is lifted a little.

Each thread shows the title its agent gave it (or the one you gave it with
`R`, kept in `ksesh_title` and shown by `ksesh agent list` too), how long ago
it last did something, and under that its session and what it needs, did or
is on. Titles
of threads that need you are bold, and a waiting or failed agent's message is
colored. A thread that finished or failed while you weren't looking is
**unread**, marked with an accent `•` at its left, until you go to its window
(the watcher turns "done" idle then, and records when it saw you in
`ksesh_seen_at`).

At the right of a thread's second line goes the branch it is on, when it is
one of its own and there is room: its name, how many commits it is ahead `↑`
and behind `↓` its base (as `ksesh wt status` counts them) and its pull
request, green when open, gray as a draft, magenta merged and red closed. What
a waiting or failed agent needs comes first: the branch only shows when both
fit. The sidebar asks git about it every 30 seconds and GitHub (`gh pr view`)
every 5 minutes, off its UI, and only while it is on screen; the pull requests
are cached in `~/.cache/ksesh` for every tab's sidebar to share. Working agents spin, a closed shelf holding the thread you're in shows
its `●`, and the status bar only offers the keys that do something for the
selected thread.

Every agent is a thread in exactly one section, the first that applies:
**Snoozed**, **Settled**, **Pinned**, **Working**, or the inbox. The shelves at
the bottom start closed. A closed shelf still shows the thread you're on, and
an open Settled shows ten threads and then a "more" row. Threads also move on
their own, the way they do in T3 Code:

- **Sending an agent a prompt** un-settles its thread and ends its snooze.
- **An agent that starts waiting on you or fails** un-settles its thread, and
  wakes it early if it is snoozed. It is then marked as woken (a magenta clock)
  until you look at it. Finishing a turn doesn't wake it, so a long job you
  snoozed stays out of the way.
- **A thread idle for 7 days settles itself** (`auto_settle_after_days`),
  unless it is pinned, snoozed, working, waiting or failed, or you took it out
  of Settled by hand since it last did something.
- **A thread whose pull request was merged settles itself** once the agent is
  idle or done, on the same terms. The sidebar does it, since it is what asks
  GitHub.
- **Pinning** un-settles and wakes a thread. **Settling** unpins and wakes
  it, and is refused while the agent is working or waiting on you.

Like the status, all of this lives in user variables on the agent's window
(`ksesh_pin`, `ksesh_snooze_until`, `ksesh_settled_at`, …), so it needs
nothing running on the side.

**Threads outlive their windows.** Claude Code names its conversation in every
hook event, and `ksesh agent hook` keeps a record of each one in
`~/.local/state/ksesh/threads/<conversation>.json` (`$XDG_STATE_HOME`): where it
runs, its session, title, last status and its triage state. Close the window,
exit Claude or quit kitty, and the thread stays in **Settled** as a *closed*
thread, muted and marked with a history icon, among the settled threads by
when it last did something. `enter` (or a click, or Resume in its menu)
resumes it: a new tab in the thread's session, which is started again if it
is gone, runs `claude --resume <id>` in the directory it ran in. The new window
is the thread again, still pinned if it was, back out of Settled. `X` forgets
a closed thread. Agents that don't name their conversation are only threads
while their window lasts.

In the sidebar:

| key          | does                                                  |
| ------------ | ----------------------------------------------------- |
| `enter` `l`  | go to the thread (or open/close a shelf, resume a closed thread), also a click |
| `esc` `h`    | back to the window you were in                        |
| `j` `k` `g` `G` `tab` | move; `tab` jumps to the next shelf          |
| `1`–`9`      | go to the nth thread of the sidebar's order (below)   |
| `n`          | new thread: asks what to do, then starts the agent (`agent_command`, default `claude`) on it in a new tab of the selected thread's session and directory, also the masthead's `+` |
| `N`          | new thread in a new worktree: asks for a branch, then what to do, and runs `ksesh wt new <branch> -p …` in the selected thread's repo |
| `p`          | pin or unpin; `K` `J` move a pin up or down            |
| `s`          | settle or un-settle                                   |
| `z`          | snooze: in 1 or 3 hours, this evening, tomorrow 9:00, next Monday 9:00, or `c` for any time (`90m`, `2d`, `15:04`, `2026-12-01 09:00`) |
| `w`          | wake a snoozed thread, or dismiss the woken mark       |
| `u`          | undo the last pin, settle, snooze or wake             |
| `m`          | the thread's menu, also a right click                 |
| `a` `x` `r`  | approve, deny or interrupt, reply, without switching  |
| `d`          | review it: `ksesh agent diff --pager` over the window beside the sidebar |
| `P`          | open its pull request in the browser (or click the `#12` under it) |
| `R`          | rename the thread; empty goes back to the agent's title |
| `X`          | close the agent's window, or forget a closed thread (asks first) |
| `A`          | archive a thread in a worktree of its own: `ksesh wt rm` removes the worktree, its branch and session, then the window closes (asks first; wt rm's refusal of uncommitted changes shows in the status bar) |
| `/`          | filter by title, session or branch as you type; `enter` keeps it, `esc` clears it |
| `o`          | group the inbox by session, like T3 Code's projects, and back |
| `f`          | only this tab's session, or every session             |
| `<` `>`      | narrow or widen the sidebar by 4 columns; new sidebars open at that width |
| `q`          | hide the sidebar in every tab                         |

The mouse works too, as in T3 Code. Hover a thread and its time gives way to
buttons, pin, snooze and settle (wake and settle on a snoozed one, restore on
a settled one), and the status bar says what the one under the mouse does.
Right click a thread, or press `m`, for a menu of everything that can be done
to it; its Snooze opens the times in its place. Below Reply… it also has
Review changes, Rename…, New thread… and New in a worktree…, and after Close
window…, for a thread in a worktree of its own, Archive worktree…. The `+` at
the right end of the masthead starts a new thread too.

```
  INBOX ───────────────────────── 2
▌  Write docs                 
▌   ap╭──────────────────────╮
      │   Go to thread    ↵ │
      │   Pin             p │
      │   Snooze          › │
      │   Settle          s │
      ├──────────────────────┤
      │   Reply…          r │
      ├──────────────────────┤
      │   Close window…   X │
      ╰──────────────────────╯
```

The same actions work from the command line, on the agent in the current
window or the one given, so they can be bound in kitty directly:

```sh
ksesh agent pin [window-id]           # toggle
ksesh agent settle [window-id]        # toggle
ksesh agent snooze 3h [window-id]     # or evening, tomorrow, next-week, 90m, 15:04…
ksesh agent wake [window-id]
ksesh sidebar print [--json]          # the threads by section, closed ones by conversation id
ksesh sidebar next / prev             # go down / up the sidebar's order
ksesh agent resume <conversation>     # resume a closed thread (an id prefix will do)
ksesh agent forget <conversation>     # drop it from the thread history
ksesh sidebar goto 3                  # go to its third thread
```

The sidebar's order, which `next`, `prev`, `goto` and the sidebar's `1`–`9`
follow, is pinned, the inbox (grouped by session after `o`), then working,
leaving out the snoozed and settled threads; after `f`, only this session's.

`ksesh agent next` skips snoozed and settled threads, and so do the tab bar's
badges and attention strip: you put those away.

Setup: the sidebar needs `listen_on` and the watcher from the
[Agents](#agents) section, which gives every tab you go to a sidebar while
it's on. Then add to `kitty.conf` (a background launch only gets kitty's
socket with `--allow-remote-control`; without it, `ksesh` can't reach kitty):

```conf
map super+b         launch --type=background --allow-remote-control ksesh sidebar toggle
map super+shift+b   launch --type=background --allow-remote-control ksesh sidebar focus
map super+ctrl+p    launch --type=background --allow-remote-control ksesh agent pin
map super+ctrl+s    launch --type=background --allow-remote-control ksesh agent settle
map super+ctrl+z    launch --type=background --allow-remote-control ksesh agent snooze 1h
map super+ctrl+w    launch --type=background --allow-remote-control ksesh agent wake
map super+ctrl+j    launch --type=background --allow-remote-control ksesh sidebar next
map super+ctrl+k    launch --type=background --allow-remote-control ksesh sidebar prev
map super+ctrl+1    launch --type=background --allow-remote-control ksesh sidebar goto 1
map super+ctrl+2    launch --type=background --allow-remote-control ksesh sidebar goto 2
# …and so on to 9
```

Full-window pickers don't fit in the sidebar. Bind their keys twice, so that
pressed in the sidebar they open over the window beside it instead:

```conf
map --when-focus-on 'not var:ksesh_role=sidebar' super+a launch --type=overlay --cwd=current ~/.config/kitty/ksesh-agents-fzf
map --when-focus-on var:ksesh_role=sidebar super+a launch --type=background --allow-remote-control ksesh sidebar overlay -- ~/.config/kitty/ksesh-agents-fzf
```

A running sidebar restarts itself into a new build of `ksesh` within a couple
of seconds of it being installed.

`super+b` shows or hides the sidebar everywhere. `super+shift+b` moves the
focus into the tab's sidebar, opening it if needed, and back out again.
Configure it in `ksesh.toml`:

```toml
[sidebar]
edge = "left"               # or "right"
width = "38"                # columns, or a share of the tab: "20%"
auto_settle_after_days = 7  # 0 never settles a thread on its own
keep_closed = 200           # closed threads kept to resume; 0 keeps none
keep_closed_days = 30       # forget closed threads idle this long; 0 never
```

Once you resize a sidebar with `<` and `>`, its width is remembered in the
sidebar's state and wins over `width`. kitty 0.50 can't resize a dock (they
sit outside the layout `resize-window` works on), so the sidebar docks a new
one at the new width and closes; sidebars already open in other tabs keep
theirs until they are opened again.

The sidebar is a window of its own, so it never counts as one of the session's
windows. A tab's sidebar closes with the tab's last window, and kitty saves it
with the tab in session files. Whether the sidebar is on, which shelves are
open and whether the inbox is grouped are shared by every tab's sidebar, in
`~/.local/state/ksesh/sidebar.json`; a filter is the one sidebar's.

The sidebars don't ask kitty what changed. The watcher's
[`ksesh_snapshot.py`](contrib/ksesh_snapshot.py) writes what they need (what
`kitten @ ls` says, less environment variables) to
`~/.local/state/ksesh/kitty-<kitty's pid>.json` a tenth of a second after
anything they show changes (a status, a title, the focus, a tab or window
opening or closing, a command starting), and every 2 seconds for the rest (the
program running in a window). The sidebars read it, and check it for a change
5 times a second, so a change shows within about a third of a second however
many tabs have one, for next to nothing (writing it takes kitty under a
millisecond). Sidebars of tabs in the background check every 5 seconds. Right
after a sidebar changes something itself, and whenever the file is missing or
more than 6 seconds old (the watcher isn't loaded, or kitty quit), they ask
kitty with `kitten @ ls` instead, every second (5 in the background), as they
did before. Other ksesh commands always ask kitty.

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
