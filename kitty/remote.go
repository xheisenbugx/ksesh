package kitty

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/xheisenbugx/ksesh/model"
)

// Agents on other machines: an agent in a `kitten ssh` window reports
// through kitty's remote control socket, which the ssh kitten forwards
// (forward_remote_control), and its hook names its machine in HostVar. An
// agent whose machine isn't this one is remote, and ksesh reaches it, and
// its directory, through ssh.

var (
	localHostOnce sync.Once
	localHost     string
)

// thisHost is this machine's hostname.
func thisHost() string {
	localHostOnce.Do(func() { localHost, _ = os.Hostname() })
	return localHost
}

// remoteHost is the machine a window's agent runs on, as ssh reaches it,
// when it isn't this one: the destination of the ssh the window runs, which
// is what ssh knows it by, else the name the agent's hook gave.
func remoteHost(w lsWindow) (string, bool) {
	named := w.UserVars[HostVar]
	if named == "" || sameHost(named, thisHost()) {
		return "", false
	}
	for i := len(w.ForegroundProcesses) - 1; i >= 0; i-- {
		if dest, ok := SSHDestination(w.ForegroundProcesses[i].Cmdline); ok {
			return dest, true
		}
	}
	if dest, ok := SSHDestination(w.Cmdline); ok {
		return dest, true
	}
	return named, true
}

// sameHost compares hostnames, short or qualified: "box" is "box.local".
func sameHost(a, b string) bool {
	short := func(h string) string { h, _, _ = strings.Cut(strings.ToLower(h), "."); return h }
	return a != "" && short(a) == short(b)
}

// sshOptsWithValue are ssh's options that take the next argument.
var sshOptsWithValue = "BbcDEeFIiJLlmOoPpQRSWw"

// SSHDestination is the host an ssh command line connects to: `ssh devbox`,
// `ssh -p 2222 me@devbox ls`, `kitten ssh --kitten cwd=/x devbox`. It
// reports false for anything that isn't ssh.
func SSHDestination(cmdline []string) (string, bool) {
	i := slices.IndexFunc(cmdline, func(arg string) bool { return filepath.Base(arg) == "ssh" || arg == "ssh" })
	if i < 0 {
		return "", false
	}
	args := cmdline[i+1:]
	for j := 0; j < len(args); j++ {
		arg := args[j]
		switch {
		case arg == "--":
			if j+1 < len(args) {
				return args[j+1], true
			}
			return "", false
		case arg == "--kitten":
			j++
		case strings.HasPrefix(arg, "--kitten="):
		case strings.HasPrefix(arg, "-") && len(arg) > 1:
			// -p 22, or -p22; a cluster like -tt takes nothing.
			if len(arg) == 2 && strings.ContainsRune(sshOptsWithValue, rune(arg[1])) {
				j++
			}
		default:
			return arg, true
		}
	}
	return "", false
}

// ThreadVars are the user variables that carry a thread's triage state,
// for a window launched with them set (see threadAssignments).
func ThreadVars(t model.Thread) map[string]string {
	vars := map[string]string{}
	for _, a := range threadAssignments(t) {
		if name, value, ok := strings.Cut(a, "="); ok {
			vars[name] = value
		}
	}
	return vars
}
