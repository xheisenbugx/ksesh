package model

// SessionFileConfig is a kitty session file (*.kitty-session) found in one of
// the configured session_file_dirs. Kitty names the session after the file.
type SessionFileConfig struct {
	Name string // file name without its extension
	Path string // absolute path to the file
}
