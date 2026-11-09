package agent

import "fmt"

// GeminiHookEvents are the Gemini CLI events `ksesh agent hook --agent
// gemini` listens to.
var GeminiHookEvents = []HookEvent{
	{"SessionStart", ""},
	{"BeforeAgent", ""},
	{"BeforeTool", ""},
	{"Notification", ""},
	{"AfterAgent", ""},
	{"SessionEnd", ""},
}

// InstallGeminiHooks adds ksesh's hooks to a Gemini CLI settings.json,
// replacing the ones a previous install left, and returns the new file. Every
// other setting and hook is kept, in the order it was in.
func InstallGeminiHooks(settings []byte, command string) ([]byte, error) {
	handler := map[string]any{
		"name":    "ksesh",
		"type":    "command",
		"command": hookCommand(command, "gemini"),
		"timeout": 5000, // milliseconds
	}
	out, err := editHooksJSON(settings, "Gemini settings", GeminiHookEvents, handler)
	return out, geminiParseHint(err)
}

// UninstallGeminiHooks removes the hooks InstallGeminiHooks added.
func UninstallGeminiHooks(settings []byte) ([]byte, error) {
	out, err := uninstallHooksJSON(settings, "Gemini settings")
	return out, geminiParseHint(err)
}

// geminiParseHint explains the likeliest reason Gemini's settings don't
// parse: Gemini CLI allows comments in them, JSON doesn't.
func geminiParseHint(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (ksesh can't edit a settings.json with comments in it)", err)
}
