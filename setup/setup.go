// Package setup installs ksesh's side of kitty next to kitty.conf: the
// watcher, tab bar and pickers from contrib/, a ksesh.conf with its settings
// and keys, written for this machine's paths, and an include of it at the
// end of kitty.conf. Run again, it brings them up to date, leaving alone what
// is already, and keeps a copy of anything it replaces that you changed.
package setup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/xheisenbugx/ksesh/contrib"
)

// Options are what the files are written for.
type Options struct {
	// KittyDir is the folder kitty.conf is in.
	KittyDir string
	// Ksesh is the absolute path of the ksesh binary kitty runs.
	Ksesh string
	// Path is the PATH kitty runs ksesh and its pickers with.
	Path string
	// Now names the backups.
	Now time.Time
}

// Change is a file setup writes, or removes when New is nil.
type Change struct {
	Path string
	// Old is what is there now, nil for no file.
	Old  []byte
	New  []byte
	Mode fs.FileMode
	// Backup keeps Old beside the file first: it is yours, or a copy of
	// ksesh's you changed.
	Backup bool
	// What says what the change is, for the user.
	What string
}

const (
	// ConfName is the kitty config setup writes, included from kitty.conf.
	ConfName = "ksesh.conf"
	// manifestName records what setup last wrote, so it can tell a file it
	// may replace from one you changed.
	manifestName = "ksesh-setup.json"
	blockStart   = "# >>> ksesh (written by ksesh setup; ksesh setup --uninstall removes it)"
	blockEnd     = "# <<< ksesh"
	sessionFile  = "sessions/main.kitty-session"
)

// executables are the files that run as programs.
var executables = map[string]bool{"ksesh-fzf": true, "ksesh-agents-fzf": true}

// copied are the files of contrib/ setup puts next to kitty.conf as they are.
var copied = []string{
	"ksesh_watcher.py", "ksesh_agents.py", "ksesh_snapshot.py", "ksesh_reload.py",
	"tab_bar.py", "ksesh-fzf", "ksesh-agents-fzf",
}

const mainSession = `# The session kitty starts in (startup_session in ksesh.conf), so no tab is
# left outside a session. ksesh lists it as "main".
cd ~
launch
`

// manifest is the hashes of the files setup wrote, by name.
type manifest struct {
	Files map[string]string `json:"files"`
}

func hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// read is a file's content, nil when it doesn't exist.
func read(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

func loadManifest(dir string) (manifest, error) {
	m := manifest{Files: map[string]string{}}
	b, err := read(filepath.Join(dir, manifestName))
	if err != nil || b == nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return manifest{Files: map[string]string{}}, nil // written again below
	}
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	return m, nil
}

// Conf is ksesh.conf, for opts.
func Conf(opts Options) ([]byte, error) {
	src, err := contrib.Files.ReadFile("ksesh.conf.tmpl")
	if err != nil {
		return nil, err
	}
	tmpl, err := template.New(ConfName).Parse(string(src))
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = tmpl.Execute(&b, map[string]any{
		"Ksesh": quote(opts.Ksesh),
		"Path":  quote(opts.Path),
		"Dir":   quote(opts.KittyDir),
		"Goto":  []int{1, 2, 3, 4, 5, 6, 7, 8, 9},
	})
	return b.Bytes(), err
}

// quote quotes s for kitty's launch, which splits its arguments the way sh
// does, unless it needs none.
func quote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+@%=:,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Install is what setup changes to install, or bring up to date, ksesh's
// side of kitty. Files already as they should be aren't in it.
func Install(opts Options) ([]Change, error) {
	m, err := loadManifest(opts.KittyDir)
	if err != nil {
		return nil, err
	}
	var changes []Change
	// A file of ksesh's: replaced as it is when it is what setup last wrote
	// (or isn't there), else kept in a backup first.
	own := func(name string, content []byte, mode fs.FileMode, what string) error {
		path := filepath.Join(opts.KittyDir, name)
		old, err := read(path)
		if err != nil {
			return err
		}
		if old != nil && bytes.Equal(old, content) {
			if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o100 == mode&0o100 {
				return nil
			}
		}
		changes = append(changes, Change{
			Path: path, Old: old, New: content, Mode: mode, What: what,
			Backup: old != nil && !bytes.Equal(old, content) && m.Files[name] != hash(old),
		})
		return nil
	}
	conf, err := Conf(opts)
	if err != nil {
		return nil, err
	}
	if err := own(ConfName, conf, 0o644, "ksesh's settings and keys"); err != nil {
		return nil, err
	}
	for _, name := range copied {
		content, err := contrib.Files.ReadFile(name)
		if err != nil {
			return nil, err
		}
		mode := fs.FileMode(0o644)
		if executables[name] {
			mode = 0o755
		}
		if err := own(name, content, mode, ""); err != nil {
			return nil, err
		}
	}

	// The session kitty starts in is yours once there: only written when
	// missing.
	session := filepath.Join(opts.KittyDir, sessionFile)
	if old, err := read(session); err != nil {
		return nil, err
	} else if old == nil {
		changes = append(changes, Change{Path: session, New: []byte(mainSession), Mode: 0o644, What: "the session kitty starts in"})
	}

	kittyConf := filepath.Join(opts.KittyDir, "kitty.conf")
	old, err := read(kittyConf)
	if err != nil {
		return nil, err
	}
	if updated := withInclude(old); !bytes.Equal(old, updated) {
		changes = append(changes, Change{Path: kittyConf, Old: old, New: updated, Mode: 0o644, Backup: old != nil, What: "includes " + ConfName})
	}

	// What setup wrote, for the next run to tell from what you changed.
	next := manifest{Files: map[string]string{}}
	next.Files[ConfName] = hash(conf)
	for _, name := range copied {
		content, _ := contrib.Files.ReadFile(name)
		next.Files[name] = hash(content)
	}
	b, _ := json.MarshalIndent(next, "", "  ")
	b = append(b, '\n')
	if path := filepath.Join(opts.KittyDir, manifestName); len(changes) > 0 || !sameFile(path, b) {
		old, _ := read(path)
		changes = append(changes, Change{Path: path, Old: old, New: b, Mode: 0o644})
	}
	return changes, nil
}

func sameFile(path string, content []byte) bool {
	old, err := read(path)
	return err == nil && bytes.Equal(old, content)
}

// Uninstall is what setup changes to take ksesh out of kitty.conf: the
// include, ksesh.conf, and the files it copied that are as it wrote them.
// The ones you changed, and the session file, stay; kept lists them.
func Uninstall(kittyDir string) (changes []Change, kept []string, err error) {
	m, err := loadManifest(kittyDir)
	if err != nil {
		return nil, nil, err
	}
	kittyConf := filepath.Join(kittyDir, "kitty.conf")
	old, err := read(kittyConf)
	if err != nil {
		return nil, nil, err
	}
	if updated := withoutInclude(old); old != nil && !bytes.Equal(old, updated) {
		changes = append(changes, Change{Path: kittyConf, Old: old, New: updated, Mode: 0o644, Backup: true, What: "no longer includes " + ConfName})
	}
	names := append([]string{ConfName}, copied...)
	for _, name := range names {
		path := filepath.Join(kittyDir, name)
		content, err := read(path)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case content == nil:
		case m.Files[name] == hash(content):
			changes = append(changes, Change{Path: path, Old: content})
		default:
			kept = append(kept, path)
		}
	}
	path := filepath.Join(kittyDir, manifestName)
	if content, err := read(path); err == nil && content != nil {
		changes = append(changes, Change{Path: path, Old: content})
	}
	return changes, kept, nil
}

// withInclude is kitty.conf with ksesh.conf included at its end, where its
// settings and keys win over kitty.conf's.
func withInclude(conf []byte) []byte {
	block := blockStart + "\ninclude " + ConfName + "\n" + blockEnd + "\n"
	rest := withoutInclude(conf)
	if bytes.Contains(conf, []byte(blockStart)) && bytes.HasSuffix(bytes.TrimRight(conf, "\n"), []byte(blockEnd)) {
		// Already there and last: leave the file exactly as it is.
		return conf
	}
	if len(bytes.TrimSpace(rest)) == 0 {
		return []byte(block)
	}
	rest = bytes.TrimRight(rest, "\n")
	return append(append(rest, "\n\n"...), block...)
}

// withoutInclude is kitty.conf without the block withInclude added.
func withoutInclude(conf []byte) []byte {
	lines := strings.SplitAfter(string(conf), "\n")
	start := slices.IndexFunc(lines, func(l string) bool { return strings.TrimSpace(l) == blockStart })
	if start < 0 {
		return conf
	}
	end := start
	for end < len(lines) && strings.TrimSpace(lines[end]) != blockEnd {
		end++
	}
	if end == len(lines) {
		return conf // no end: leave it to the user
	}
	before := lines[:start]
	if n := len(before); n > 0 && strings.TrimSpace(before[n-1]) == "" {
		before = before[:n-1]
	}
	return []byte(strings.Join(before, "") + strings.Join(lines[end+1:], ""))
}

// Apply makes the changes, keeping a backup of the files that ask for one as
// <file>.ksesh-<time>.bak, and returns the backups' paths by file.
func Apply(changes []Change, now time.Time) (map[string]string, error) {
	backups := map[string]string{}
	for _, c := range changes {
		if c.Backup && c.Old != nil {
			backup, err := keep(c.Path, c.Old, now)
			if err != nil {
				return backups, err
			}
			backups[c.Path] = backup
		}
		if c.New == nil {
			if err := os.Remove(c.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return backups, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
			return backups, err
		}
		if err := writeAtomic(c.Path, c.New, c.Mode); err != nil {
			return backups, err
		}
	}
	return backups, nil
}

// writeAtomic replaces a file in one step: kitty re-reads some of these the
// moment they change, and must never see half of one.
func writeAtomic(path string, content []byte, mode fs.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(content)
	if err := errors.Join(werr, tmp.Chmod(mode), tmp.Close()); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// keep copies a file's old content to a backup of a name not taken yet.
func keep(path string, old []byte, now time.Time) (string, error) {
	stamp := fmt.Sprintf("%s.ksesh-%s", path, now.Format("20060102-150405"))
	for i := 0; i < 100; i++ {
		backup := stamp + ".bak"
		if i > 0 {
			backup = fmt.Sprintf("%s-%d.bak", stamp, i)
		}
		f, err := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("couldn't back up %s: %w", path, err)
		}
		_, werr := f.Write(old)
		if err := errors.Join(werr, f.Close()); err != nil {
			return "", fmt.Errorf("couldn't back up %s: %w", path, err)
		}
		return backup, nil
	}
	return "", fmt.Errorf("couldn't back up %s: too many backups", path)
}

// KittyDir is the folder kitty reads kitty.conf from, as kitty finds it:
// $KITTY_CONFIG_DIRECTORY, else $XDG_CONFIG_HOME/kitty or ~/.config/kitty,
// else on macOS ~/Library/Preferences/kitty if kitty.conf is there.
func KittyDir(getenv func(string) string, home string, goos string) string {
	if dir := getenv("KITTY_CONFIG_DIRECTORY"); dir != "" {
		return dir
	}
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "kitty")
	}
	dir := filepath.Join(home, ".config", "kitty")
	if goos == "darwin" {
		mac := filepath.Join(home, "Library", "Preferences", "kitty")
		if _, err := os.Stat(filepath.Join(dir, "kitty.conf")); err != nil {
			if _, err := os.Stat(filepath.Join(mac, "kitty.conf")); err == nil {
				return mac
			}
		}
	}
	return dir
}

// LaunchPath is the PATH kitty runs ksesh with: ksesh's own folder, then the
// absolute folders of path (the PATH of the shell setup runs in), then the
// system's, each once.
func LaunchPath(ksesh, path string) string {
	var dirs []string
	add := func(d string) {
		if d = filepath.Clean(d); filepath.IsAbs(d) && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	add(filepath.Dir(ksesh))
	for d := range strings.SplitSeq(path, string(os.PathListSeparator)) {
		add(d)
	}
	for _, d := range []string{"/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		add(d)
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}
