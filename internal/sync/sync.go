package sync

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aayushgautam/wtm/internal/config"
	"github.com/aayushgautam/wtm/internal/gitx"
	"github.com/bmatcuk/doublestar/v4"
)

type planItem struct {
	srcAbs string
	rel    string // slash-separated
	dstAbs string
}

type skipError struct {
	dst string
}

func (e skipError) Error() string {
	return "skipped " + e.dst
}

func Run(args []string) error {
	fsFlags := flag.NewFlagSet("sync", flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard) // we'll print our own usage/errors

	var repoHint string
	var worktreeNum int
	var destOverride string
	var yes bool
	var force bool

	fsFlags.StringVar(&repoHint, "repo", "", "repo path (defaults to current dir repo)")
	fsFlags.IntVar(&worktreeNum, "worktree", 0, "worktree number (1-indexed)")
	fsFlags.StringVar(&destOverride, "dest", "", "destination worktree path")
	fsFlags.BoolVar(&yes, "yes", false, "skip global proceed confirmation")
	fsFlags.BoolVar(&force, "force", false, "overwrite files without per-file prompting")

	if err := fsFlags.Parse(args); err != nil {
		return usageError(err)
	}

	repoRoot, err := gitx.RepoRoot(repoHint)
	if err != nil {
		return err
	}

	wts, err := gitx.ListWorktrees(repoRoot)
	if err != nil {
		return err
	}

	destRoot, err := pickWorktree(repoRoot, wts, destOverride, worktreeNum)
	if err != nil {
		return err
	}
	if samePath(repoRoot, destRoot) {
		return fmt.Errorf("selected worktree is the current repo root; nothing to sync")
	}

	loaded, err := config.Load(repoRoot)
	if err != nil {
		return err
	}

	plan, err := buildPlan(repoRoot, destRoot, loaded.Config)
	if err != nil {
		return err
	}

	printWork(repoRoot, destRoot, loaded.Source, plan)

	if len(plan) == 0 {
		fmt.Fprintln(os.Stderr, "No files matched; nothing to do.")
		return nil
	}

	if !yes {
		if !confirm("Proceed? [y/N] ") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	copied := 0
	skipped := 0

	for _, it := range plan {
		if err := copyOne(it.srcAbs, it.dstAbs, force); err != nil {
			// Non-fatal: keep going but report.
			var se skipError
			if errors.As(err, &se) {
				fmt.Fprintln(os.Stderr, "Skipped:", se.dst)
				skipped++
				continue
			}
			fmt.Fprintln(os.Stderr, "Error:", err.Error())
			skipped++
			continue
		}
		copied++
	}

	fmt.Fprintf(os.Stderr, "Done. Copied: %d, skipped: %d\n", copied, skipped)
	return nil
}

func usageError(err error) error {
	msg := strings.TrimSpace(err.Error())
	if msg != "" {
		fmt.Fprintln(os.Stderr, "Error:", msg)
	}
	fmt.Fprintln(os.Stderr, "usage: wtm sync [--repo PATH] [--worktree N | --dest PATH] [--yes] [--force]")
	return fmt.Errorf("invalid arguments")
}

func pickWorktree(repoRoot string, wts []gitx.Worktree, destOverride string, worktreeNum int) (string, error) {
	if destOverride != "" {
		for _, wt := range wts {
			if samePath(wt.Path, destOverride) {
				return wt.Path, nil
			}
		}
		return "", fmt.Errorf("--dest did not match an active worktree path: %s", destOverride)
	}

	if worktreeNum != 0 {
		if worktreeNum < 1 || worktreeNum > len(wts) {
			return "", fmt.Errorf("--worktree must be between 1 and %d", len(wts))
		}
		return wts[worktreeNum-1].Path, nil
	}

	fmt.Fprintln(os.Stderr, "Active worktrees:")
	for i, wt := range wts {
		branchLabel := "(detached)"
		if wt.Branch != "" {
			branchLabel = strings.TrimPrefix(wt.Branch, "refs/heads/")
		}
		headShort := wt.Head
		if len(headShort) > 8 {
			headShort = headShort[:8]
		}
		fmt.Fprintf(os.Stderr, "  [%d] %s  %s  %s\n", i+1, wt.Path, branchLabel, headShort)
	}

	for {
		s := prompt("Select worktree number to sync into: ")
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err == nil && n >= 1 && n <= len(wts) {
			return wts[n-1].Path, nil
		}
		fmt.Fprintf(os.Stderr, "Invalid selection. Enter a number between 1 and %d.\n", len(wts))
	}
}

func buildPlan(repoRoot, destRoot string, cfg config.Config) ([]planItem, error) {
	repoRoot = filepath.Clean(repoRoot)
	destRoot = filepath.Clean(destRoot)

	include := normalizePatterns(cfg.Include)
	exclude := normalizePatterns(cfg.Exclude)

	var items []planItem

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Cheap fast-path skips for huge dirs.
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		relOS, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)

		if !matchesAny(include, rel) {
			return nil
		}
		if matchesAny(exclude, rel) {
			return nil
		}

		dst := filepath.Join(destRoot, relOS)
		if samePath(path, dst) {
			return nil
		}

		items = append(items, planItem{
			srcAbs: path,
			rel:    rel,
			dstAbs: dst,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Stable order for predictable previews.
	sortPlan(items)
	return items, nil
}

func normalizePatterns(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, filepath.ToSlash(p))
	}
	return out
}

func matchesAny(patterns []string, rel string) bool {
	for _, p := range patterns {
		ok, err := doublestar.Match(p, rel)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func sortPlan(items []planItem) {
	// Avoid pulling in a dependency; simple insertion sort is fine for small lists.
	// If this grows large, we can switch to sort.Slice.
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j-1].rel > items[j].rel {
			items[j-1], items[j] = items[j], items[j-1]
			j--
		}
	}
}

func printWork(repoRoot, destRoot, configSource string, plan []planItem) {
	fmt.Fprintln(os.Stderr, "Repo:", repoRoot)
	fmt.Fprintln(os.Stderr, "Dest:", destRoot)
	fmt.Fprintln(os.Stderr, "Config:", configSource)
	fmt.Fprintf(os.Stderr, "Planned copies: %d\n", len(plan))
	for _, it := range plan {
		fmt.Fprintf(os.Stdout, "%s -> %s\n", it.srcAbs, it.dstAbs)
	}
}

func prompt(msg string) string {
	fmt.Fprint(os.Stderr, msg)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

func confirm(msg string) bool {
	s := strings.ToLower(strings.TrimSpace(prompt(msg)))
	return s == "y" || s == "yes"
}

func copyOne(src, dst string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}

	if _, err := os.Stat(dst); err == nil {
		if !force {
			if !confirm(fmt.Sprintf("Overwrite %s? [y/N] ", dst)) {
				return skipError{dst: dst}
			}
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dst, err)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat %s: %w", src, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}

	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		return fmt.Errorf("chmod %s: %w", dst, err)
	}

	mtime := srcInfo.ModTime()
	atime := time.Now()
	_ = os.Chtimes(dst, atime, mtime) // best-effort

	return nil
}

func samePath(a, b string) bool {
	aa := filepath.Clean(a)
	bb := filepath.Clean(b)
	return aa == bb
}

