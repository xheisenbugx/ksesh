// Package review gathers what an agent has done in its worktree, and renders
// it for a terminal: the branch against its base, its commits, the files it
// touched, and the diff.
//
// "Done" is everything since the branch left its base: commits, uncommitted
// changes and new files alike, the way a reviewer sees the branch, rather than
// only the uncommitted part (`git diff HEAD`) that misses everything an agent
// has already committed.
package review

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xheisenbugx/ksesh/shell"
)

// Limits on what new, untracked files contribute to the diff: their content is
// read to show them, and a generated or vendored directory could be huge.
const (
	MaxUntrackedFiles = 30
	MaxUntrackedBytes = 256 << 10
	MaxUntrackedLines = 400
	MaxCommits        = 15
)

type Commit struct {
	Hash    string
	Subject string
	Age     string
}

type File struct {
	Path      string
	OldPath   string // for a rename
	Status    string // A, M, D, R, ? (untracked)
	Added     int
	Deleted   int
	Binary    bool
	Truncated bool // only part of an untracked file is in the patch
	// Uncommitted is set when the file has changes not committed yet, as
	// opposed to only changes the branch's commits made.
	Uncommitted bool
}

type Review struct {
	Path   string
	Branch string // empty when detached
	// Base is what the branch is compared to: origin's default branch, or the
	// upstream when the branch is the default branch itself. Empty means
	// only uncommitted changes are shown (against HEAD).
	Base      string
	MergeBase string
	Ahead     int
	Behind    int
	Commits   []Commit
	Files     []File
	// Patch is a unified diff of everything in Files, untracked files
	// included as new ones.
	Patch string
}

type collector struct {
	sh   shell.Shell
	path string
}

// gitConfig pins what a user's git config could change about the output
// parsed here: quoted paths, and the diff prefixes (diff.noprefix,
// diff.mnemonicPrefix) parsePatch reads the paths from.
var gitConfig = []string{"-c", "core.quotePath=false", "-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false", "-c", "diff.external="}

func (c collector) git(args ...string) (string, error) {
	return c.sh.Cmd("git", append(append([]string{"-C", c.path}, gitConfig...), args...)...)
}

// Collect reads the review of the worktree at path. base overrides the ref to
// compare against.
func Collect(sh shell.Shell, path string, base string) (Review, error) {
	// The whole worktree, even when the agent runs in a subdirectory of it:
	// diff paths are relative to the top, and so are untracked ones read here.
	top, err := sh.Cmd("git", "-C", path, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) == "" {
		return Review{Path: path}, fmt.Errorf("%s is not in a git worktree", path)
	}
	path = strings.TrimSpace(top)
	c := collector{sh, path}
	r := Review{Path: path}
	if branch, err := c.git("symbolic-ref", "--quiet", "--short", "HEAD"); err == nil {
		r.Branch = strings.TrimSpace(branch)
	}
	r.Base = base
	if r.Base == "" {
		r.Base = c.defaultBase(r.Branch)
	}
	from := "HEAD"
	if r.Base != "" {
		if mb, err := c.git("merge-base", "HEAD", r.Base); err == nil {
			r.MergeBase = strings.TrimSpace(mb)
			from = r.MergeBase
		}
		if counts, err := c.git("rev-list", "--left-right", "--count", "HEAD..."+r.Base); err == nil {
			fmt.Sscanf(strings.TrimSpace(counts), "%d %d", &r.Ahead, &r.Behind)
		}
		if log, err := c.git("log", "--no-color", "-n", strconv.Itoa(MaxCommits), "--format=%h%x09%s%x09%cr", from+"..HEAD"); err == nil {
			r.Commits = parseLog(log)
		}
	}

	numstat, _ := c.git("diff", "--no-color", "-M", "--numstat", from)
	nameStatus, _ := c.git("diff", "--no-color", "-M", "--name-status", from)
	r.Files = parseFiles(nameStatus, numstat)
	patch, _ := c.git("diff", "--no-color", "--no-ext-diff", "--src-prefix=a/", "--dst-prefix=b/", "-M", "--histogram", from)

	if status, err := c.git("status", "--porcelain=v1", "--untracked-files=no"); err == nil {
		dirty := map[string]bool{}
		for _, line := range nonEmpty(status) {
			if len(line) > 3 {
				name := line[3:]
				if _, renamed, ok := strings.Cut(name, " -> "); ok {
					name = renamed
				}
				dirty[strings.Trim(name, `"`)] = true
			}
		}
		for i := range r.Files {
			r.Files[i].Uncommitted = dirty[r.Files[i].Path]
		}
	}

	var untracked strings.Builder
	others, _ := c.git("ls-files", "--others", "--exclude-standard")
	for i, rel := range nonEmpty(others) {
		f := File{Path: rel, Status: "?", Uncommitted: true}
		if i < MaxUntrackedFiles {
			f = untrackedPatch(&untracked, filepath.Join(path, rel), f)
		} else {
			f.Truncated = true
		}
		r.Files = append(r.Files, f)
	}
	r.Patch = strings.TrimRight(patch, "\n")
	if untracked.Len() > 0 {
		if r.Patch != "" {
			r.Patch += "\n"
		}
		r.Patch += strings.TrimRight(untracked.String(), "\n")
	}
	return r, nil
}

// defaultBase is the ref a branch is reviewed against: origin's default
// branch, or for the default branch itself what isn't pushed yet.
func (c collector) defaultBase(branch string) string {
	def, err := c.git("symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	def = strings.TrimSpace(def)
	if err != nil || def == "" {
		for _, candidate := range []string{"origin/main", "origin/master", "origin/develop", "origin/dev"} {
			if _, err := c.git("rev-parse", "--verify", "--quiet", candidate); err == nil {
				def = candidate
				break
			}
		}
	}
	if def != "" && branch != "" && branch != strings.TrimPrefix(def, "origin/") {
		return def
	}
	if upstream, err := c.git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}"); err == nil {
		return strings.TrimSpace(upstream)
	}
	return ""
}

func parseLog(out string) []Commit {
	var commits []Commit
	for _, line := range nonEmpty(out) {
		parts := strings.SplitN(line, "\t", 3)
		for len(parts) < 3 {
			parts = append(parts, "")
		}
		commits = append(commits, Commit{Hash: parts[0], Subject: parts[1], Age: parts[2]})
	}
	return commits
}

// parseFiles pairs `git diff --name-status` with `--numstat`, which list the
// same files in the same order.
func parseFiles(nameStatus, numstat string) []File {
	var files []File
	for _, line := range nonEmpty(nameStatus) {
		fields := strings.Split(line, "\t")
		f := File{Status: fields[0][:1], Path: fields[len(fields)-1]}
		if f.Status == "R" || f.Status == "C" {
			f.OldPath = fields[1]
		}
		files = append(files, f)
	}
	for i, line := range nonEmpty(numstat) {
		if i >= len(files) {
			break
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "-" {
			files[i].Binary = true
			continue
		}
		files[i].Added, _ = strconv.Atoi(fields[0])
		files[i].Deleted, _ = strconv.Atoi(fields[1])
	}
	return files
}

// untrackedPatch writes a new file as a unified diff against nothing, so it
// reads like any other added file.
func untrackedPatch(out *strings.Builder, abs string, f File) File {
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() {
		return f
	}
	if info.Size() > MaxUntrackedBytes {
		f.Truncated = true
		fmt.Fprintf(out, "diff --git a/%[1]s b/%[1]s\nnew file mode 100644\nBinary files /dev/null and b/%[1]s differ\n", f.Path)
		return f
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return f
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		f.Binary = true
		fmt.Fprintf(out, "diff --git a/%[1]s b/%[1]s\nnew file mode 100644\nBinary files /dev/null and b/%[1]s differ\n", f.Path)
		return f
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(data) == 0 {
		lines = nil
	}
	f.Added = len(lines)
	shown := lines
	if len(shown) > MaxUntrackedLines {
		shown, f.Truncated = shown[:MaxUntrackedLines], true
	}
	fmt.Fprintf(out, "diff --git a/%[1]s b/%[1]s\nnew file mode 100644\n--- /dev/null\n+++ b/%[1]s\n", f.Path)
	if len(shown) > 0 {
		fmt.Fprintf(out, "@@ -0,0 +1,%d @@\n", len(shown))
		for _, l := range shown {
			out.WriteString("+" + l + "\n")
		}
	}
	return f
}

func nonEmpty(out string) []string {
	var lines []string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimRight(line, "\r"))
		}
	}
	return lines
}
