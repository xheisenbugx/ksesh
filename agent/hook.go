package agent

// HookAgents are the agents `ksesh agent hook --agent <name>` reads the hook
// events of.
var HookAgents = []string{"claude", "codex", "gemini"}

// FromHook maps a hook event of the named agent onto a status update. ok is
// false for an event that changes nothing, or an agent ksesh can't read.
func FromHook(agent string, raw []byte) (Update, bool) {
	switch agent {
	case "", "claude":
		return FromClaudeHook(raw)
	case "codex":
		return FromCodexHook(raw)
	case "gemini":
		return FromGeminiHook(raw)
	}
	return Update{}, false
}
