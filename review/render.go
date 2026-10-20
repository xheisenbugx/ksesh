package review

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Colors, Catppuccin Mocha like the rest of ksesh's kitty side. The line
// backgrounds are the ones Catppuccin's delta theme uses.
var (
	fgText    = rgb(38, "#cdd6f4")
	fgDim     = rgb(38, "#6c7086")
	fgGreen   = rgb(38, "#a6e3a1")
	fgRed     = rgb(38, "#f38ba8")
	fgYellow  = rgb(38, "#f9e2af")
	fgBlue    = rgb(38, "#89b4fa")
	fgMauve   = rgb(38, "#cba6f7")
	fgTeal    = rgb(38, "#94e2d5")
	bgAdd     = rgb(48, "#394545")
	bgDel     = rgb(48, "#493447")
	bgFile    = rgb(48, "#313244")
	bold      = "\x1b[1m"
	reset     = "\x1b[0m"
	tabSpaces = "    "
)

func rgb(layer int, hex string) string {
	n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", layer, n>>16&0xff, n>>8&0xff, n&0xff)
}

// Options shape a rendered review.
type Options struct {
	Width int
	// StatOnly leaves the diff out: branch, commits and files only.
	StatOnly bool
	// MaxFileLines collapses the diff of a file with more changed lines than
	// this to its header (0: no limit), so a generated file doesn't bury the
	// rest in a preview.
	MaxFileLines int
}

// Render draws a review: a summary of the branch, its commits and files, then
// the diff.
func Render(r Review, opts Options) string {
	width := max(opts.Width, 40)
	var b strings.Builder
	renderSummary(&b, r, width)
	if !opts.StatOnly && r.Patch != "" {
		b.WriteString("\n")
		RenderPatch(&b, r.Patch, r.Files, width, opts.MaxFileLines)
	}
	return strings.TrimRight(b.String(), "\n")
}

func renderSummary(b *strings.Builder, r Review, width int) {
	added, deleted := 0, 0
	for _, f := range r.Files {
		added += f.Added
		deleted += f.Deleted
	}
	branch := r.Branch
	if branch == "" {
		branch = "(detached)"
	}
	line := bold + fgMauve + branch + reset
	if r.Base != "" {
		line += fgDim + " → " + reset + fgBlue + r.Base + reset
		line += "  " + fgGreen + fmt.Sprintf("↑%d", r.Ahead) + reset + " " + fgRed + fmt.Sprintf("↓%d", r.Behind) + reset
	} else {
		line += fgDim + "  uncommitted changes" + reset
	}
	fmt.Fprintln(b, line)
	if len(r.Files) == 0 {
		fmt.Fprintln(b, fgDim+"no changes"+reset)
		return
	}
	uncommitted := 0
	for _, f := range r.Files {
		if f.Uncommitted {
			uncommitted++
		}
	}
	line = fmt.Sprintf("%s%d file%s%s  %s+%d%s %s−%d%s", fgDim, len(r.Files), plural(len(r.Files)), reset,
		fgGreen, added, reset, fgRed, deleted, reset)
	if uncommitted > 0 {
		line += fmt.Sprintf("  %s● %d not committed%s", fgYellow, uncommitted, reset)
	}
	fmt.Fprintln(b, line)

	if len(r.Commits) > 0 {
		b.WriteString("\n")
		for _, c := range r.Commits {
			subject := ansi.Truncate(c.Subject, width-len(c.Hash)-len(c.Age)-4, "…")
			fmt.Fprintf(b, "%s%s%s %s %s%s%s\n", fgYellow, c.Hash, reset, subject, fgDim, c.Age, reset)
		}
		if more := r.Ahead - len(r.Commits); more > 0 {
			fmt.Fprintf(b, "%s… %d more%s\n", fgDim, more, reset)
		}
	}

	b.WriteString("\n")
	pathWidth := 0
	for _, f := range r.Files {
		pathWidth = max(pathWidth, ansi.StringWidth(fileLabel(f)))
	}
	pathWidth = min(pathWidth, width-16)
	for _, f := range r.Files {
		label := ansi.Truncate(fileLabel(f), pathWidth, "…")
		mark := " "
		if f.Uncommitted {
			mark = fgYellow + "●" + reset
		}
		fmt.Fprintf(b, "%s%s %s%s  %s\n", statusBadge(f.Status), mark, label,
			strings.Repeat(" ", max(pathWidth-ansi.StringWidth(label), 0)), counts(f))
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func fileLabel(f File) string {
	if f.OldPath != "" {
		return f.OldPath + " → " + f.Path
	}
	return f.Path
}

func statusBadge(status string) string {
	switch status {
	case "A":
		return fgGreen + "A" + reset
	case "D":
		return fgRed + "D" + reset
	case "R", "C":
		return fgMauve + status + reset
	case "?":
		return fgTeal + "?" + reset
	}
	return fgYellow + "M" + reset
}

func counts(f File) string {
	if f.Binary {
		return fgDim + "binary" + reset
	}
	var parts []string
	if f.Added > 0 {
		parts = append(parts, fgGreen+"+"+strconv.Itoa(f.Added)+reset)
	}
	if f.Deleted > 0 {
		parts = append(parts, fgRed+"−"+strconv.Itoa(f.Deleted)+reset)
	}
	if f.Status == "?" {
		parts = append(parts, fgDim+"new"+reset)
	}
	if f.Truncated {
		parts = append(parts, fgDim+"(cut)"+reset)
	}
	return strings.Join(parts, " ")
}

// fileDiff is one file of a unified diff.
type fileDiff struct {
	path, oldPath string
	status        string
	binary        bool
	hunks         []hunk
}

type hunk struct {
	oldStart, newStart int
	context            string
	lines              []string // with their +, - or space prefix
}

func parsePatch(patch string) []fileDiff {
	var files []fileDiff
	var cur *fileDiff
	inHeader := false
	for line := range strings.SplitSeq(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			files = append(files, fileDiff{status: "M"})
			cur = &files[len(files)-1]
			inHeader = true
			if _, b, ok := strings.Cut(line, " b/"); ok {
				cur.path = b
			}
		case cur == nil:
			continue
		case inHeader:
			switch {
			case strings.HasPrefix(line, "new file mode"):
				cur.status = "A"
			case strings.HasPrefix(line, "deleted file mode"):
				cur.status = "D"
			case strings.HasPrefix(line, "rename from "):
				cur.status, cur.oldPath = "R", strings.TrimPrefix(line, "rename from ")
			case strings.HasPrefix(line, "rename to "):
				cur.path = strings.TrimPrefix(line, "rename to ")
			case strings.HasPrefix(line, "Binary files"):
				cur.binary = true
			case strings.HasPrefix(line, "+++ b/"):
				cur.path = strings.TrimPrefix(line, "+++ b/")
			case strings.HasPrefix(line, "@@"):
				inHeader = false
				cur.hunks = append(cur.hunks, parseHunkHeader(line))
			}
		case strings.HasPrefix(line, "@@"):
			cur.hunks = append(cur.hunks, parseHunkHeader(line))
		case len(cur.hunks) > 0:
			h := &cur.hunks[len(cur.hunks)-1]
			h.lines = append(h.lines, line)
		}
	}
	return files
}

// parseHunkHeader reads "@@ -12,7 +12,9 @@ func name()".
func parseHunkHeader(line string) hunk {
	var h hunk
	rest := strings.TrimPrefix(line, "@@ ")
	ranges, context, _ := strings.Cut(rest, " @@")
	h.context = strings.TrimSpace(context)
	for _, r := range strings.Fields(ranges) {
		start, _, _ := strings.Cut(r[1:], ",")
		n, _ := strconv.Atoi(start)
		if r[0] == '-' {
			h.oldStart = n
		} else {
			h.newStart = n
		}
	}
	return h
}

// RenderPatch draws a unified diff: a bar per file, then its hunks with line
// numbers, additions and deletions on tinted backgrounds that run the full
// width, and long lines wrapped rather than cut.
func RenderPatch(b *strings.Builder, patch string, files []File, width int, maxFileLines int) {
	stats := map[string]File{}
	for _, f := range files {
		stats[f.Path] = f
	}
	for i, fd := range parsePatch(patch) {
		if i > 0 {
			b.WriteString("\n")
		}
		f := stats[fd.path]
		f.Path, f.OldPath, f.Binary = fd.path, fd.oldPath, f.Binary || fd.binary
		if f.Status == "" || (f.Status == "M" && fd.status != "M") {
			f.Status = fd.status
		}
		// Every reset inside the bar has to bring its background back.
		onBar := func(s string) string { return strings.ReplaceAll(s, reset, reset+bgFile) }
		title := " " + onBar(statusBadge(f.Status)) + " " + bold + fgText + fileLabel(f) + reset + bgFile + "  " + onBar(counts(f))
		b.WriteString(fill(bgFile+title, width) + reset + "\n")
		if f.Binary {
			b.WriteString(fgDim + "     binary file" + reset + "\n")
			continue
		}
		if n := changedLines(fd); maxFileLines > 0 && n > maxFileLines {
			fmt.Fprintf(b, "%s     %d changed lines, folded here (the full review shows them)%s\n", fgDim, n, reset)
			continue
		}
		for _, h := range fd.hunks {
			sep := "  ⋯"
			if h.context != "" {
				sep += " " + h.context
			}
			b.WriteString(fgBlue + ansi.Truncate(sep, width, "…") + reset + "\n")
			oldN, newN := h.oldStart, h.newStart
			for _, l := range h.lines {
				if l == "" {
					l = " "
				}
				marker, content := l[0], strings.ReplaceAll(l[1:], "\t", tabSpaces)
				switch marker {
				case '+':
					writeLine(b, newN, "+", content, fgGreen, bgAdd, width)
					newN++
				case '-':
					writeLine(b, oldN, "-", content, fgRed, bgDel, width)
					oldN++
				case '\\':
					b.WriteString(fgDim + "       " + strings.TrimSpace(l[1:]) + reset + "\n")
				default:
					writeLine(b, newN, " ", content, "", "", width)
					oldN++
					newN++
				}
			}
		}
	}
}

func changedLines(fd fileDiff) int {
	n := 0
	for _, h := range fd.hunks {
		for _, l := range h.lines {
			if l != "" && (l[0] == '+' || l[0] == '-') {
				n++
			}
		}
	}
	return n
}

// gutterWidth is the line number column: "1234 + ".
const gutterWidth = 7

func writeLine(b *strings.Builder, n int, marker, content, markerFg, bg string, width int) {
	textWidth := max(width-gutterWidth, 10)
	pieces := strings.Split(ansi.Hardwrap(content, textWidth, true), "\n")
	for i, piece := range pieces {
		gutter := fmt.Sprintf("%4d ", n)
		if i > 0 {
			gutter = "   ↪ "
		}
		line := fgDim + gutter + reset + bg + markerFg + marker + " " + reset + bg + fgText + piece
		if bg != "" {
			line = fill(line, width)
		}
		b.WriteString(line + reset + "\n")
	}
}

// fill pads a styled line with spaces to width, so its background runs to the
// edge.
func fill(line string, width int) string {
	if pad := width - ansi.StringWidth(line); pad > 0 {
		return line + strings.Repeat(" ", pad)
	}
	return line
}
