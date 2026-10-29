package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xheisenbugx/ksesh/execwrap"
	"github.com/xheisenbugx/ksesh/home"
	"github.com/xheisenbugx/ksesh/oswrap"
	"github.com/xheisenbugx/ksesh/shell"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return sgr.ReplaceAllString(s, "") }

const samplePatch = `diff --git a/app.go b/app.go
index 1111111..2222222 100644
--- a/app.go
+++ b/app.go
@@ -10,4 +10,5 @@ func main() {
 	start()
-	stop()
+	pause()
+	resume()
 }
--- a/not/a/header
\ No newline at end of file
diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
diff --git a/logo.png b/logo.png
new file mode 100644
Binary files /dev/null and b/logo.png differ`

func TestParsePatch(t *testing.T) {
	files := parsePatch(samplePatch)
	require.Len(t, files, 3)

	app := files[0]
	assert.Equal(t, "app.go", app.path)
	assert.Equal(t, "M", app.status)
	require.Len(t, app.hunks, 1)
	assert.Equal(t, 10, app.hunks[0].oldStart)
	assert.Equal(t, 10, app.hunks[0].newStart)
	assert.Equal(t, "func main() {", app.hunks[0].context)
	assert.Contains(t, app.hunks[0].lines, "--- a/not/a/header", "inside a hunk, --- is a deleted line")

	assert.Equal(t, "R", files[1].status)
	assert.Equal(t, "old.txt", files[1].oldPath)
	assert.Equal(t, "new.txt", files[1].path)

	assert.Equal(t, "A", files[2].status)
	assert.True(t, files[2].binary)
}

func TestRenderPatchNumbersAndWraps(t *testing.T) {
	var b strings.Builder
	RenderPatch(&b, samplePatch, []File{{Path: "app.go", Status: "M", Added: 2, Deleted: 1}}, 60, 0)
	out := plain(b.String())

	assert.Contains(t, out, "M app.go  +2 −1")
	assert.Contains(t, out, "  ⋯ func main() {")
	assert.Contains(t, out, "  10       start()")
	assert.Contains(t, out, "  11 -     stop()")
	assert.Contains(t, out, "  11 +     pause()")
	assert.Contains(t, out, "  12 +     resume()")
	assert.Contains(t, out, "R old.txt → new.txt")
	assert.Contains(t, out, "binary file")
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 60, line)
	}

	var long strings.Builder
	RenderPatch(&long, "diff --git a/x b/x\n@@ -1 +1 @@\n+"+strings.Repeat("y", 80), nil, 40, 0)
	assert.Contains(t, plain(long.String()), "   ↪ ", "long lines wrap instead of being cut")
}

func TestRenderPatchFoldsBigFiles(t *testing.T) {
	var b strings.Builder
	RenderPatch(&b, samplePatch, nil, 80, 2)
	out := plain(b.String())
	assert.Contains(t, out, "4 changed lines, folded")
	assert.NotContains(t, out, "pause()")
}

func TestParseFilesPairsStatusAndCounts(t *testing.T) {
	files := parseFiles("M\tapp.go\nR087\told.txt\tnew.txt\nA\tlogo.png", "2\t1\tapp.go\n0\t0\tnew.txt\n-\t-\tlogo.png")
	require.Len(t, files, 3)
	assert.Equal(t, File{Path: "app.go", Status: "M", Added: 2, Deleted: 1}, files[0])
	assert.Equal(t, File{Path: "new.txt", OldPath: "old.txt", Status: "R"}, files[1])
	assert.True(t, files[2].Binary)
}

func TestCollectSeesCommitsUncommittedAndNewFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	write := func(name, content string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
	run("init", "-q", "-b", "main")
	write("app.go", "package app\n")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	run("checkout", "-q", "-b", "feat/x")
	write("app.go", "package app\n\nfunc A() {}\n")
	run("commit", "-q", "-am", "add A")
	write("app.go", "package app\n\nfunc A() {}\nfunc B() {}\n")
	write("docs/new.md", "# hi\nthere\n")

	sh := shell.NewShell(execwrap.NewExec(), home.NewHome(oswrap.NewOs()))
	r, err := Collect(sh, filepath.Join(dir), "main")
	require.NoError(t, err)

	assert.Equal(t, "feat/x", r.Branch)
	assert.Equal(t, "main", r.Base)
	assert.Equal(t, 1, r.Ahead)
	require.Len(t, r.Commits, 1)
	assert.Equal(t, "add A", r.Commits[0].Subject)
	require.Len(t, r.Files, 2)
	assert.Equal(t, File{Path: "app.go", Status: "M", Added: 3, Uncommitted: true}, r.Files[0], "committed and uncommitted changes together")
	assert.Equal(t, File{Path: "docs/new.md", Status: "?", Added: 2, Uncommitted: true}, r.Files[1])
	assert.Contains(t, r.Patch, "+func B() {}")
	assert.Contains(t, r.Patch, "+++ b/docs/new.md\n@@ -0,0 +1,2 @@\n+# hi\n+there")

	out := plain(Render(r, Options{Width: 80}))
	assert.Contains(t, out, "feat/x → main  ↑1 ↓0")
	assert.Contains(t, out, "2 files  +5 −0  ● 2 not committed")
	assert.Contains(t, out, "? docs/new.md")

	_, err = Collect(sh, t.TempDir(), "")
	assert.Error(t, err, "not a git worktree")
}
