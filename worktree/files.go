package worktree

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// IncludeFile lists files to copy from the main worktree into a new one, the
// file Claude Code, Conductor and others read for the same thing. ksesh reads
// it as a list of paths and globs relative to the repo root (filepath.Match
// syntax, no "**" or "!" negations), one per line, "#" starting a comment.
const IncludeFile = ".worktreeinclude"

// populate copies and symlinks the files a new worktree needs from the main
// one: the ones gitignore keeps out of a checkout, such as .env or a
// node_modules too big to reinstall. Nothing is overwritten, so a file the
// branch tracks wins over the main worktree's copy.
func (w *RealWorktree) populate(repo repoContext, target string) error {
	copies := append(readIncludeFile(filepath.Join(repo.main, IncludeFile)), repo.cfg.Copy...)
	var errs []error
	for _, rel := range expand(repo.main, copies) {
		if err := copyPath(filepath.Join(repo.main, rel), filepath.Join(target, rel)); err != nil {
			errs = append(errs, err)
		}
	}
	for _, rel := range expand(repo.main, repo.cfg.Symlink) {
		dst := filepath.Join(target, rel)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Symlink(filepath.Join(repo.main, rel), dst); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func readIncludeFile(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// expand resolves patterns against root into the relative paths that exist,
// each once. Patterns that try to leave root are ignored.
func expand(root string, patterns []string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, pattern := range patterns {
		pattern = filepath.Clean(strings.TrimPrefix(strings.TrimSuffix(pattern, "/"), "/"))
		if pattern == "." || strings.HasPrefix(pattern, "..") {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			continue
		}
		for _, m := range matches {
			rel, err := filepath.Rel(root, m)
			if err != nil || strings.HasPrefix(rel, "..") || rel == ".git" || seen[rel] {
				continue
			}
			seen[rel] = true
			paths = append(paths, rel)
		}
	}
	return paths
}

// copyPath copies a file, or a directory recursively, without overwriting
// anything already at dst.
func copyPath(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		out := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(out, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			if _, err := os.Lstat(out); err == nil {
				return nil
			}
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, out)
		case info.Mode().IsRegular():
			return copyFile(path, out, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copying %s: %w", src, err)
	}
	return out.Close()
}
