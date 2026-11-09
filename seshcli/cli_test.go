package seshcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/dir"
	"github.com/xheisenbugx/ksesh/focuser"
	"github.com/xheisenbugx/ksesh/git"
	"github.com/xheisenbugx/ksesh/github"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/json"
	"github.com/xheisenbugx/ksesh/model"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/pathwrap"
	"github.com/xheisenbugx/ksesh/replacer"
	"github.com/xheisenbugx/ksesh/runtimewrap"
	"github.com/xheisenbugx/ksesh/shell"
	"github.com/xheisenbugx/ksesh/threads"
)

// cli runs ksesh's commands end to end, through buildDeps, against a kitty
// that only exists as a mocked shell: every `kitten @` call ksesh makes has
// to be expected on sh, so none reaches a real kitty. Its config, state and
// cache live in temp directories, and the kitty environment of whoever runs
// the tests is replaced by the test's own.
type cli struct {
	sh    *shell.MockShell
	base  *BaseDeps
	state string // $XDG_STATE_HOME
	// config is the --config file, empty for none.
	config string
}

// newCLI is ksesh run outside kitty's windows (as a background launch is),
// talking to the kitty `kitten @ ls` answers ls for.
func newCLI(t *testing.T, ls string) *cli {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("KITTY_LISTEN_ON", "")
	t.Setenv("KITTY_PID", "")

	sh := shell.NewMockShell(t)
	if ls != "" {
		sh.EXPECT().CmdCapture("kitten", "@", "ls").Return(ls, nil).Maybe()
	}
	o, p, rt := oswrap.NewOs(), pathwrap.NewPath(), runtimewrap.NewRunTime()
	g := git.NewGit(sh)
	return &cli{
		sh:    sh,
		state: state,
		base: &BaseDeps{
			Os: o, Path: p, Runtime: rt, Home: home.NewHome(o), Shell: sh,
			Json: json.NewJson(), Replacer: replacer.NewReplacer(),
			Git: g, Github: github.NewGithub(sh, g), Focuser: focuser.NewFocuser(rt, sh),
			Dir: dir.NewDir(o, g, p),
		},
	}
}

// withConfig has the commands read this ksesh.toml.
func (c *cli) withConfig(t *testing.T, toml string) *cli {
	t.Helper()
	c.config = filepath.Join(t.TempDir(), "ksesh.toml")
	require.NoError(t, os.WriteFile(c.config, []byte(toml), 0o644))
	return c
}

// run runs `ksesh <args>` and returns what it printed.
func (c *cli) run(args ...string) (stdout, stderr string, err error) {
	root := &cobra.Command{Use: "ksesh", SilenceUsage: true, SilenceErrors: true, TraverseChildren: true}
	root.PersistentFlags().StringP("config", "C", "", "path to config file")
	root.AddCommand(NewSidebarCommand(c.base), NewAgentCommand(c.base), NewWorktreeCommand(c.base))
	if c.config != "" {
		args = append([]string{"--config", c.config}, args...)
	}
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

// store is the thread history the commands read.
func (c *cli) store() *threads.Store {
	return threads.NewStore(filepath.Join(c.state, "ksesh", "threads"))
}

// kitten expects one `kitten @ <args>` call.
func (c *cli) kitten(out string, args ...any) {
	c.sh.EXPECT().CmdCapture("kitten", append([]any{"@"}, args...)...).Return(out, nil).Once()
}

// win is a window of a `kitten @ ls` fixture.
type win struct {
	id      int
	session string
	title   string
	cwd     string
	active  bool
	focused float64 // last_focused_at
	vars    map[string]string
	program string // the foreground process, when it isn't a hook-reporting agent
}

type tab struct {
	id     int
	title  string
	active bool
	wins   []win
}

// kittyLs renders a single focused OS window holding tabs as `kitten @ ls`
// prints it.
func kittyLs(tabs ...tab) string {
	var tabsJSON []string
	for _, t := range tabs {
		var winsJSON []string
		for _, w := range t.wins {
			var vars []string
			for k, v := range w.vars {
				vars = append(vars, strconv.Quote(k)+":"+strconv.Quote(v))
			}
			fg := ""
			if w.program != "" {
				fg = `,"foreground_processes":[{"cmdline":[` + strconv.Quote(w.program) + `],"cwd":` + strconv.Quote(w.cwd) + `}]`
			}
			winsJSON = append(winsJSON, fmt.Sprintf(
				`{"id":%d,"is_active":%t,"is_focused":%t,"session_name":%q,"title":%q,"cwd":%q,"last_focused_at":%g,"user_vars":{%s}%s}`,
				w.id, w.active, w.active && t.active, w.session, w.title, w.cwd, w.focused, strings.Join(vars, ","), fg))
		}
		tabsJSON = append(tabsJSON, fmt.Sprintf(`{"id":%d,"is_active":%t,"is_focused":%t,"title":%q,"windows":[%s]}`,
			t.id, t.active, t.active, t.title, strings.Join(winsJSON, ",")))
	}
	return `[{"id":1,"is_active":true,"is_focused":true,"tabs":[` + strings.Join(tabsJSON, ",") + `]}]`
}

// sidebarWin is a tab's docked sidebar.
func sidebarWin(id int, session string, focused float64, active bool) win {
	return win{id: id, session: session, cwd: "/", focused: focused, active: active, vars: map[string]string{"ksesh_role": "sidebar"}}
}

// The times threadsLs's threads did things at, a minute boundary away from
// how print rounds them, so that a test running slowly prints the same.
var (
	fixtureNow         = time.Now()
	fixtureSnoozeUntil = time.Unix(fixtureNow.Add(2*time.Hour).Unix(), 0)
)

func ago(d time.Duration) string { return strconv.FormatInt(fixtureNow.Add(-d).Unix(), 10) }

// threadsLs is kitty with an agent in every section of the sidebar:
//
//	tab 10 (api): 1 done 2m ago, 2 waiting 5m ago, 9 the sidebar
//	tab 11 (web): 3 failed 1m ago, 4 snoozed, 5 settled, 6 pinned,
//	              7 working, 8 a plain shell
//
// active is the window you are in; its tab is the one in front.
func threadsLs(active int) string {
	in := func(ids ...int) bool { return slices.Contains(ids, active) }
	agentAt := func(status string, d time.Duration, extra ...string) map[string]string {
		vars := map[string]string{"ksesh_agent": "claude", "ksesh_status": status, "ksesh_status_at": ago(d)}
		for i := 0; i+1 < len(extra); i += 2 {
			vars[extra[i]] = extra[i+1]
		}
		return vars
	}
	return kittyLs(
		tab{id: 10, title: "api", active: in(1, 2, 9), wins: []win{
			{id: 1, session: "api", title: "✳ Fix the login", cwd: "/c/api", active: active == 1, focused: 50,
				vars: agentAt("done", 2*time.Minute+30*time.Second, "ksesh_conv", "conv-1")},
			{id: 2, session: "api", title: "✳ Claude Code", cwd: "/c/api", active: active == 2, focused: 40,
				vars: agentAt("waiting", 5*time.Minute+30*time.Second, "ksesh_msg", "Allow Bash", "ksesh_title", "Migrations")},
			sidebarWin(9, "api", 60, active == 9),
		}},
		tab{id: 11, title: "web", active: in(3, 4, 5, 6, 7, 8), wins: []win{
			{id: 3, session: "web", title: "✳ Lint", cwd: "/c/web", active: active == 3, focused: 30,
				vars: agentAt("error", time.Minute+30*time.Second)},
			{id: 4, session: "web", title: "✳ Later", cwd: "/c/web", active: active == 4, focused: 20,
				vars: agentAt("done", time.Hour, "ksesh_snooze_until", strconv.FormatInt(fixtureSnoozeUntil.Unix(), 10), "ksesh_snoozed_at", ago(time.Hour))},
			{id: 5, session: "web", title: "✳ Old", cwd: "/c/web", active: active == 5, focused: 10,
				vars: agentAt("done", 3*time.Hour+30*time.Minute, "ksesh_settled_at", ago(time.Hour+30*time.Minute))},
			{id: 6, session: "web", title: "✳ Keep", cwd: "/c/web", active: active == 6, focused: 5,
				vars: agentAt("idle", 4*time.Hour+30*time.Minute, "ksesh_pin", "1")},
			{id: 7, session: "web", title: "✳ Build", cwd: "/c/web", active: active == 7, focused: 4,
				vars: agentAt("working", 3*time.Minute+30*time.Second)},
			{id: 8, session: "web", cwd: "/c/web", active: active == 8, focused: 3, program: "-zsh"},
		}},
	)
}

// closedRecord puts a closed thread, one whose window is gone, in the
// history.
func closedRecord(t *testing.T, c *cli, conv string, change func(r *model.ThreadRecord)) {
	t.Helper()
	require.NoError(t, c.store().Update(conv, func(r *model.ThreadRecord) {
		at := fixtureNow.Add(-time.Hour - 30*time.Minute)
		r.Agent, r.Status, r.StatusAt, r.UpdatedAt = "claude", "done", at, at
		if change != nil {
			change(r)
		}
	}))
}
