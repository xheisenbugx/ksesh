# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

ksesh is a port of sesh (a smart terminal session manager written in Go) to kitty: it creates and switches kitty's native sessions instead of tmux sessions, using zoxide for directories. Binary: `ksesh`.

## Core Architecture

- **Module**: `github.com/xheisenbugx/ksesh`
- **Go Version**: 1.26.0
- **Main Entry Point**: `main.go` → `seshcli.App()`

### Key Packages

- `seshcli/` - CLI commands and dependency injection (`deps.go` wires all dependencies)
- `lister/` - Lists sessions from various sources (kitty, zoxide, config, sessionfile)
- `connector/` - Handles session connections with type-specific strategies
- `namer/` - Session naming with strategy pattern (git bare → git → directory fallback)
- `configurator/` - Configuration management (`ksesh.toml` in `$XDG_CONFIG_HOME/ksesh`, falling back to sesh's `sesh/sesh.toml`)
- `kitty/` - kitty backend over `kitten @` remote control. Sessions are derived from `kitten @ ls` (windows sharing a `session_name`); tmux windows map to kitty tabs and panes to kitty windows. `freeze.go` explains why the picker snapshots kitty in tty mode.
- `agent/` - Coding agent status (working/waiting/done/error/idle): Claude Code, Codex and Gemini CLI hook mapping (`claude.go`, `codex.go`, `gemini.go`) and installers, next/last selection, hook install into Claude settings. Status lives in kitty user variables on the agent's window (`kitty/agent.go`); `contrib/ksesh_agents.py` draws it in kitty's tab bar and sends notifications
- `sidebar/` - The thread sidebar (`ksesh sidebar`): a bubbletea TUI in a kitty `tab-dock` window (kitty 0.50+) listing agents as T3 Code-style threads. Sections and the pin/snooze/settle rules are pure functions in `agent/thread.go`; triage state is more user variables on the agent's window. Sidebar windows carry `ksesh_role=sidebar` and are kept out of sessions
- `threads/` - The thread history: one JSON record per agent conversation in `$XDG_STATE_HOME/ksesh/threads/`, written by `ksesh agent hook`, so threads outlive their windows as closed threads that can be resumed. `threads.Kitty` wraps the kitty backend for the sidebar, adding closed threads to `ListAgents`
- `setup/` - `ksesh setup` (`seshcli/setup.go`): installs kitty's side next to kitty.conf from the files `contrib/embed.go` embeds, writes `ksesh.conf` from `contrib/ksesh.conf.tmpl` with this machine's paths and includes it at the end of kitty.conf. `ksesh-setup.json` there records what it wrote, so a file the user changed is backed up before being replaced. `just install` builds and runs it
- `contrib/ksesh_snapshot.py` - Loaded by the watcher: kitty writes its `ls` state to `$XDG_STATE_HOME/ksesh/kitty-<pid>.json` on every relevant event, and sidebars read that (`kitty/snapshot.go`) instead of polling `kitten @ ls`
- `sessionfile/` - Finds kitty session files (the tmuxinator counterpart)
- `zoxide/` - Frecency backend integration
- `model/` - Shared data structures (Config, SeshSession, ConnectOpts)

### Dependency Injection Pattern

Dependencies are wired in `seshcli/deps.go`:
- `BaseDeps` - Config-free dependencies (exec, os, path, shell wrappers)
- `Deps` - Config-dependent dependencies (lister, connector, namer, etc.)
- `NewBaseDeps()` → `BuildAll(configPath)` creates the full dependency graph

### Naming Strategy

The `namer` package determines session names using a strategy chain:
1. Git bare repository name (looks for `.bare` folder)
2. Git repository name (from remote or directory)
3. Directory name (respects `dir_length` config)

## Common Development Commands

### Using justfile (preferred)
```bash
just install        # Build to ~/.local/bin/ksesh and run ksesh setup (kitty config, hooks)
just build          # Build to $GOPATH/bin/ksesh
just build v2.0.0   # Build with specific version
just test           # Run tests with coverage (regenerates mocks first)
just mock           # Generate mocks only
```

### Direct commands
```bash
go test -run TestFunctionName ./package/...  # Run single test
go test -cover -race ./namer/...             # Test specific package
```

### Generate mocks
Mocks are configured in `.mockery.yaml` and placed alongside interfaces:
```bash
just mock
# Or: GOFLAGS="-buildvcs=false" go tool mockery
```

## Configuration

- Config file: `$XDG_CONFIG_HOME/sesh/sesh.toml` or `~/.config/sesh/sesh.toml`
- Custom path: `sesh --config /path/to/sesh.toml <command>`
- Test config examples: `configurator/testdata/`

## Development Notes

- Always prefer `just` commands over raw `go` commands when a justfile recipe exists
- All external dependencies use interfaces for testability (see wrapper packages: `execwrap`, `oswrap`, `pathwrap`)
- Mock files follow pattern `mock_*.go` in the same package as the interface
- Follow existing patterns for error handling and `slog` logging
