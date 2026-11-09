package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// HookMarker is in every hook command ksesh installs, which is how a later
// install or uninstall finds its own entries among the user's other hooks.
const HookMarker = "agent hook"

// HookEvent is an agent's hook event ksesh listens to, with its matcher (""
// for none).
type HookEvent struct {
	Event   string
	Matcher string
}

// ClaudeHookEvents are the Claude Code events `ksesh agent hook` listens to.
var ClaudeHookEvents = []HookEvent{
	{"SessionStart", ""},
	{"UserPromptSubmit", ""},
	{"PostToolUse", ""},
	{"PostToolUseFailure", ""},
	{"PermissionRequest", ""},
	{"PermissionDenied", ""},
	{"Notification", "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input"},
	{"Stop", ""},
	{"StopFailure", ""},
	{"SessionEnd", ""},
}

// InstallClaudeHooks adds ksesh's hooks to a Claude Code settings.json,
// replacing the ones a previous install left, and returns the new file. Every
// other setting and hook is kept, in the order it was in. command is how the
// hook runs ksesh, normally the absolute path of the binary.
func InstallClaudeHooks(settings []byte, command string) ([]byte, error) {
	handler := map[string]any{
		"type":    "command",
		"command": command + " " + HookMarker,
		"timeout": 5,
	}
	return editHooksJSON(settings, "Claude settings", ClaudeHookEvents, handler)
}

// UninstallClaudeHooks removes the hooks InstallClaudeHooks added.
func UninstallClaudeHooks(settings []byte) ([]byte, error) {
	return editHooksJSON(settings, "Claude settings", nil, nil)
}

// editHooksJSON rewrites the "hooks" object of a JSON settings file, laid out
// the way Claude Code, Codex and Gemini CLI all read it ({event: [{matcher,
// hooks: [handler]}]}): it drops the matcher groups ksesh added before, then
// appends one running handler to each of events. Appending leaves the user's
// own groups where they were, which matters to Codex: it trusts a hook by its
// position. what names the file in errors.
func editHooksJSON(settings []byte, what string, events []HookEvent, handler map[string]any) ([]byte, error) {
	root := &object{}
	if len(bytes.TrimSpace(settings)) > 0 {
		if err := json.Unmarshal(settings, root); err != nil {
			return nil, fmt.Errorf("couldn't parse %s: %w", what, err)
		}
	}
	hooks := &object{}
	if raw, ok := root.get("hooks"); ok {
		if err := json.Unmarshal(raw, hooks); err != nil {
			return nil, fmt.Errorf("couldn't parse the hooks in %s: %w", what, err)
		}
	}
	if err := removeOwnEntries(hooks, what); err != nil {
		return nil, err
	}
	for _, ev := range events {
		entry := map[string]any{"hooks": []map[string]any{handler}}
		if ev.Matcher != "" {
			entry["matcher"] = ev.Matcher
		}
		raw, err := marshal(entry)
		if err != nil {
			return nil, err
		}
		if err := appendEntry(hooks, ev.Event, raw, what); err != nil {
			return nil, err
		}
	}
	raw, err := marshal(hooks)
	if err != nil {
		return nil, err
	}
	if len(hooks.keys) == 0 {
		root.delete("hooks")
	} else {
		root.set("hooks", raw)
	}
	out, err := marshal(root)
	if err != nil {
		return nil, err
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, out, "", "  "); err != nil {
		return nil, err
	}
	indented.WriteByte('\n')
	return indented.Bytes(), nil
}

// hookEntry is one matcher group of a hook event.
type hookEntry struct {
	Hooks []struct {
		Command string `json:"command"`
	} `json:"hooks"`
}

// removeOwnEntries drops every matcher group whose hooks are all ksesh's, and
// every event left with no group.
func removeOwnEntries(hooks *object, what string) error {
	for _, event := range append([]string(nil), hooks.keys...) {
		raw, _ := hooks.get(event)
		var entries []json.RawMessage
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("couldn't parse the %s hooks in %s: %w", event, what, err)
		}
		kept := entries[:0]
		for _, e := range entries {
			if !isOwnEntry(e) {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			hooks.delete(event)
			continue
		}
		if len(kept) != len(entries) {
			out, err := marshal(kept)
			if err != nil {
				return err
			}
			hooks.set(event, out)
		}
	}
	return nil
}

func isOwnEntry(raw json.RawMessage) bool {
	var e hookEntry
	if err := json.Unmarshal(raw, &e); err != nil || len(e.Hooks) == 0 {
		return false
	}
	for _, h := range e.Hooks {
		if !IsOwnHookCommand(h.Command) {
			return false
		}
	}
	return true
}

// IsOwnHookCommand reports whether a hook command is one ksesh installed:
// `<ksesh> agent hook` for Claude Code, `<ksesh> agent hook --agent <name>`
// for the others.
func IsOwnHookCommand(command string) bool {
	if !strings.Contains(command, "ksesh") {
		return false
	}
	if strings.HasSuffix(command, HookMarker) {
		return true
	}
	_, name, ok := strings.Cut(command, HookMarker+" --agent ")
	return ok && name != "" && !strings.ContainsAny(name, " ;&|")
}

func appendEntry(hooks *object, event string, entry json.RawMessage, what string) error {
	var entries []json.RawMessage
	if raw, ok := hooks.get(event); ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("couldn't parse the %s hooks in %s: %w", event, what, err)
		}
	}
	out, err := marshal(append(entries, entry))
	if err != nil {
		return err
	}
	hooks.set(event, out)
	return nil
}

// object is a JSON object that keeps its keys in order, so rewriting someone's
// settings file doesn't shuffle it.
type object struct {
	keys   []string
	values map[string]json.RawMessage
}

func (o *object) get(key string) (json.RawMessage, bool) {
	v, ok := o.values[key]
	return v, ok
}

func (o *object) set(key string, value json.RawMessage) {
	if o.values == nil {
		o.values = map[string]json.RawMessage{}
	}
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *object) delete(key string) {
	if _, ok := o.values[key]; !ok {
		return
	}
	delete(o.values, key)
	for i, k := range o.keys {
		if k == key {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *object) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected a JSON object")
	}
	*o = object{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		o.set(key, value)
	}
	_, err = dec.Token()
	return err
}

func (o object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(o.values[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshal is json.Marshal without HTML escaping, which would rewrite every
// "&&" in the user's own hook commands as "\u0026\u0026".
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
