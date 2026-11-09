package agent

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// CodexHookEvents are the Codex events `ksesh agent hook --agent codex`
// listens to. SessionStart leaves out compaction, which restarts the session
// mid-turn.
var CodexHookEvents = []HookEvent{
	{"SessionStart", "startup|resume|clear|fork"},
	{"UserPromptSubmit", ""},
	{"PermissionRequest", ""},
	{"PostToolUse", ""},
	{"PostModelRequest", ""},
	{"Stop", ""},
	{"Interrupt", ""},
	{"SessionEnd", ""},
}

// codexTimeout is in seconds. Interrupt and SessionEnd hooks may take at
// most 3, and the others need far less than that.
const codexTimeout = 3

func codexHandler(command string) map[string]any {
	return map[string]any{
		"type":    "command",
		"command": hookCommand(command, "codex"),
		"timeout": codexTimeout,
	}
}

// hookCommand is the shell command an agent's hooks run ksesh with.
func hookCommand(command, agent string) string {
	return shellQuote(command) + " " + HookMarker + " --agent " + agent
}

// shellQuote quotes a path for sh, unless it needs none.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+~@%=:,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// CodexFiles are the two files Codex reads hooks from in its home: hooks.json
// and inline [hooks] tables in config.toml. nil means the file doesn't exist,
// or, as a result, that it is to be left as it is.
type CodexFiles struct {
	HooksJSON  []byte
	ConfigTOML []byte
}

// InstallCodexHooks adds ksesh's hooks to Codex, replacing the ones a previous
// install left, and returns the files that change. They go in hooks.json,
// unless config.toml already has hooks of its own: Codex warns at every start
// when one folder has both, so they then join those, in a block of their own
// at the end of config.toml. Nothing else in either file changes, and the
// user's hooks keep their place, which is what Codex trusts them by.
func InstallCodexHooks(in CodexFiles, command string) (CodexFiles, error) {
	config, err := removeCodexTOMLBlock(in.ConfigTOML)
	if err != nil {
		return CodexFiles{}, err
	}
	inline, err := codexHasInlineHooks(config)
	if err != nil {
		return CodexFiles{}, err
	}
	out := CodexFiles{ConfigTOML: config}
	if !inline {
		out.HooksJSON, err = editHooksJSON(in.HooksJSON, "Codex hooks.json", CodexHookEvents, codexHandler(command))
		if err != nil {
			return CodexFiles{}, err
		}
		if in.ConfigTOML == nil {
			out.ConfigTOML = nil
		}
		return out, nil
	}
	if out.HooksJSON, err = uninstallHooksJSON(in.HooksJSON, "Codex hooks.json"); err != nil {
		return CodexFiles{}, err
	}
	out.ConfigTOML = appendCodexTOMLBlock(config, command)
	var check map[string]any
	if err := toml.Unmarshal(out.ConfigTOML, &check); err != nil {
		return CodexFiles{}, fmt.Errorf("couldn't add ksesh's hooks to the [hooks] in Codex's config.toml (%w); "+
			"move your hooks to hooks.json, or add `ksesh agent hook --agent codex` to them by hand", err)
	}
	return out, nil
}

// UninstallCodexHooks removes the hooks InstallCodexHooks added, from either
// file.
func UninstallCodexHooks(in CodexFiles) (CodexFiles, error) {
	config, err := removeCodexTOMLBlock(in.ConfigTOML)
	if err != nil {
		return CodexFiles{}, err
	}
	hooks, err := uninstallHooksJSON(in.HooksJSON, "Codex hooks.json")
	if err != nil {
		return CodexFiles{}, err
	}
	return CodexFiles{HooksJSON: hooks, ConfigTOML: config}, nil
}

// CodexHooksDisabled reports whether config.toml turns Codex's hooks off.
func CodexHooksDisabled(configTOML []byte) bool {
	var config struct {
		Features map[string]any `toml:"features"`
	}
	if toml.Unmarshal(configTOML, &config) != nil {
		return false
	}
	for _, key := range []string{"hooks", "codex_hooks"} {
		if on, ok := config.Features[key].(bool); ok && !on {
			return true
		}
	}
	return false
}

// uninstallHooksJSON drops ksesh's hooks from a JSON hooks file, leaving a
// file that has none exactly as it is.
func uninstallHooksJSON(settings []byte, what string) ([]byte, error) {
	if !bytes.Contains(settings, []byte(HookMarker)) {
		return settings, nil
	}
	return editHooksJSON(settings, what, nil, nil)
}

// codexHasInlineHooks reports whether config.toml defines hooks of its own.
// [hooks.state] doesn't count: it is where Codex keeps which hooks you
// trusted, whichever file they are in.
func codexHasInlineHooks(configTOML []byte) (bool, error) {
	var config struct {
		Hooks map[string]any `toml:"hooks"`
	}
	if err := toml.Unmarshal(configTOML, &config); err != nil {
		return false, fmt.Errorf("couldn't parse Codex's config.toml: %w", err)
	}
	for key := range config.Hooks {
		if key != "state" {
			return true, nil
		}
	}
	return false, nil
}

const (
	codexBlockStart = "# >>> ksesh agent hooks (ksesh agent uninstall --codex removes them)"
	codexBlockEnd   = "# <<< ksesh agent hooks"
)

// appendCodexTOMLBlock adds ksesh's hooks to the end of config.toml, between
// marker comments. They are arrays of tables, so they extend the user's own
// [[hooks.<event>]] arrays wherever those are.
func appendCodexTOMLBlock(config []byte, command string) []byte {
	var b bytes.Buffer
	b.Write(config)
	if len(config) > 0 && !bytes.HasSuffix(config, []byte("\n")) {
		b.WriteByte('\n')
	}
	if len(bytes.TrimSpace(config)) > 0 {
		b.WriteByte('\n')
	}
	quoted, _ := marshal(hookCommand(command, "codex")) // a JSON string is a TOML basic string
	b.WriteString(codexBlockStart + "\n")
	for _, ev := range CodexHookEvents {
		fmt.Fprintf(&b, "[[hooks.%s]]\n", ev.Event)
		if ev.Matcher != "" {
			m, _ := marshal(ev.Matcher)
			fmt.Fprintf(&b, "matcher = %s\n", m)
		}
		fmt.Fprintf(&b, "[[hooks.%s.hooks]]\ntype = \"command\"\ncommand = %s\ntimeout = %d\n\n", ev.Event, quoted, codexTimeout)
	}
	b.Truncate(b.Len() - 1)
	b.WriteString(codexBlockEnd + "\n")
	return b.Bytes()
}

// ownTOMLHeader is a table header appendCodexTOMLBlock writes.
var ownTOMLHeader = regexp.MustCompile(`^\[\[hooks\.[A-Za-z]+(\.hooks)?\]\]$`)

// removeCodexTOMLBlock takes out the block appendCodexTOMLBlock added. Codex
// itself edits config.toml, recording the hooks you trust in [hooks.state]
// tables, and may put one inside the block: any table in it that isn't one of
// ksesh's hooks stays.
func removeCodexTOMLBlock(config []byte) ([]byte, error) {
	if config == nil {
		return nil, nil
	}
	lines := strings.SplitAfter(string(config), "\n")
	start, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case codexBlockStart:
			start = i
		case codexBlockEnd:
			if start >= 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if start < 0 {
		return config, nil
	}
	if end < 0 {
		return nil, fmt.Errorf("Codex's config.toml has the start of ksesh's hooks but not their end (%q); remove them by hand", codexBlockEnd)
	}
	// Keep every table in the block that isn't ksesh's, from its header to
	// the next one.
	var kept []string
	own := true
	for _, line := range lines[start+1 : end] {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			own = ownTOMLHeader.MatchString(trimmed)
		}
		if !own {
			kept = append(kept, line)
		}
	}
	before := lines[:start]
	// The blank line appendCodexTOMLBlock put before the block.
	if n := len(before); n > 0 && strings.TrimSpace(before[n-1]) == "" {
		before = before[:n-1]
	}
	out := strings.Join(before, "") + strings.Join(kept, "") + strings.Join(lines[end+1:], "")
	return []byte(out), nil
}
