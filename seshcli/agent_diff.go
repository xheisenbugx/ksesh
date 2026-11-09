package seshcli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/xheisenbugx/ksesh/review"
)

func newAgentDiffCommand(base *BaseDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <window-id>",
		Short: "Review what an agent has done: its branch, commits, files and diff",
		Long: `Review the worktree an agent works in, the way its branch will be reviewed:
everything since it left its base (origin's default branch, or the upstream
for the default branch itself), commits, uncommitted changes and new files
alike, not only what isn't committed yet.

It prints the branch against its base, the commits, the files with their
added and deleted lines, and the diff, drawn with line numbers and tinted
backgrounds. With delta (https://github.com/dandavison/delta) on the PATH the
diff goes through it instead, for syntax highlighting. The width is the fzf
preview's, the terminal's, or --width.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := buildDeps(cmd, base)
			if err != nil {
				return err
			}
			a, err := findAgent(deps, args[0])
			if err != nil {
				return err
			}
			baseRef, _ := cmd.Flags().GetString("base")
			statOnly, _ := cmd.Flags().GetBool("stat")
			noDelta, _ := cmd.Flags().GetBool("no-delta")
			width, _ := cmd.Flags().GetInt("width")
			if width <= 0 {
				width = outputWidth()
			}

			r, err := review.Collect(deps.Shell, a.Path, baseRef)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			var paged bytes.Buffer
			pager, _ := cmd.Flags().GetBool("pager")
			if pager {
				out = &paged
			}
			delta, _ := exec.LookPath("delta")
			maxLines, _ := cmd.Flags().GetInt("max-file-lines")
			opts := review.Options{Width: width, StatOnly: statOnly, MaxFileLines: maxLines}
			if statOnly || noDelta || delta == "" || r.Patch == "" {
				fmt.Fprintln(out, review.Render(r, opts))
			} else {
				opts.StatOnly = true
				fmt.Fprintln(out, review.Render(r, opts))
				fmt.Fprintln(out)
				d := exec.Command(delta, "--paging=never", "--line-numbers", "--width="+strconv.Itoa(width))
				d.Stdin = strings.NewReader(r.Patch + "\n")
				d.Stdout, d.Stderr = out, cmd.ErrOrStderr()
				if err := d.Run(); err != nil {
					return err
				}
			}
			if pager {
				return page(paged.String(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			}
			return nil
		},
	}
	cmd.Flags().Bool("pager", false, "page it in less, as the sidebar's d does")
	cmd.Flags().String("base", "", "ref to compare against (default: origin's default branch)")
	cmd.Flags().Bool("stat", false, "only the summary: branch, commits and files")
	cmd.Flags().Bool("no-delta", false, "draw the diff with ksesh even when delta is installed")
	cmd.Flags().Int("max-file-lines", 0, "fold the diff of files with more changed lines than this (0: never)")
	cmd.Flags().IntP("width", "w", 0, "columns to draw in (default: fzf preview or terminal width)")
	return cmd
}

// page shows text in less, keeping its colors, or prints it without less.
func page(text string, out, errOut io.Writer) error {
	less, err := exec.LookPath("less")
	if err != nil {
		_, err := io.WriteString(out, text)
		return err
	}
	// -R keeps the colors; -+F undoes an F in $LESS, which would quit at once
	// on a review that fits on screen, closing an overlay before you read it.
	p := exec.Command(less, "-R", "-+F")
	p.Stdin = strings.NewReader(text)
	p.Stdout, p.Stderr = out, errOut
	return p.Run()
}

// outputWidth is the fzf preview's width when ksesh draws one, the
// terminal's otherwise.
func outputWidth() int {
	if cols, err := strconv.Atoi(os.Getenv("FZF_PREVIEW_COLUMNS")); err == nil && cols > 0 {
		return cols
	}
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 100
}
