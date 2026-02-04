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

const (
	storeRootDir = ".wtm"
	storeSubDir  = "configs"
)

type planItem struct {
	rel         string
	repoAbs     string
	storeAbs    string
	worktreeAbs string
}

type skipError struct {
	dst string
}

type syncOptions struct {
	repoHint     string
	worktreeNum  int
	destOverride string
	yes          bool
	force        bool
}

func (e skipError) Error() string {
	return "skipped " + e.dst
}

func Run(args []string) error {
	opts, err := parseOptions("sync", args)
	if err != nil {
		return usageError("sync", err)
	}

	repoRoot, err := gitx.RepoRoot(opts.repoHint)
	if err != nil {
		return err
	}

	wts, err := gitx.ListWorktrees(repoRoot)
	if err != nil {
		return err
	}

	worktree, err := pickWorktree(repoRoot, wts, opts.destOverride, opts.worktreeNum)
	if err != nil {
		return err
	}
	destRoot := worktree.Path
	if samePath(repoRoot, destRoot) {
		return fmt.Errorf("selected worktree is the current repo root; nothing to sync")
	}

	loaded, err := config.Load(repoRoot)
	if err != nil {
		return err
	}

	storeRoot, err := storeRootPath(repoRoot, worktree)
	if err != nil {
		return err
	}

	plan, err := buildSyncPlan(repoRoot, destRoot, storeRoot, loaded.Config)
	if err != nil {
		return err
	}

	printSyncPlan(repoRoot, destRoot, storeRoot, loaded.Source, plan)

	if len(plan) == 0 {
		fmt.Fprintln(os.Stderr, "No files matched; nothing to do.")
		return nil
	}

	if !opts.yes {
		if !confirm("Proceed? [y/N] ") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	copied := 0
	linked := 0
	skipped := 0

	for _, it := range plan {
		if err := copyRepoToStore(it.repoAbs, it.storeAbs); err != nil {
			fmt.Fprintln(os.Stderr, "Error copying to store:", err)
			skipped++
			continue
		}
		copied++

		if err := ensureWorktreeLink(it.storeAbs, it.worktreeAbs, opts.force); err != nil {
			var se skipError
			if errors.As(err, &se) {
				fmt.Fprintln(os.Stderr, "Skipped:", se.dst)
				skipped++
				continue
			}
			fmt.Fprintln(os.Stderr, "Error symlinking:", err)
			skipped++
			continue
		}
		linked++
	}

	fmt.Fprintf(os.Stderr, "Done. Copied into store: %d, linked: %d, skipped: %d\n", copied, linked, skipped)
	return nil
}

func Push(args []string) error {
	opts, err := parseOptions("push", args)
	if err != nil {
		return usageError("push", err)
	}

	repoRoot, err := gitx.RepoRoot(opts.repoHint)
	if err != nil {
		return err
	}

	wts, err := gitx.ListWorktrees(repoRoot)
	if err != nil {
		return err
	}

	worktree, err := pickWorktree(repoRoot, wts, opts.destOverride, opts.worktreeNum)
	if err != nil {
		return err
	}

	loaded, err := config.Load(repoRoot)
	if err != nil {
		return err
	}

	storeRoot, err := storeRootPath(repoRoot, worktree)
	if err != nil {
		return err
	}

	if _, err := os.Stat(storeRoot); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("store %s does not exist; run \"wtm sync\" first", storeRoot)
		}
		return err
	}

	plan, err := buildPushPlan(storeRoot, repoRoot, loaded.Config)
	if err != nil {
		return err
	}

	printPushPlan(repoRoot, storeRoot, loaded.Source, plan)

	if len(plan) == 0 {
		fmt.Fprintln(os.Stderr, "No files in store match the configured include/exclude patterns.")
		return nil
	}

	if !opts.yes {
		if !confirm("Proceed? [y/N] ") {
			fmt.Fprintln(os.Stderr, "Aborted.")
			return nil
		}
	}

	pushed := 0
	skipped := 0

	for _, it := range plan {
		if err := copyStoreToRepo(it.storeAbs, it.repoAbs, opts.force); err != nil {
			var se skipError
			if errors.As(err, &se) {
				fmt.Fprintln(os.Stderr, "Skipped:", se.dst)
				skipped++
				continue
			}
			fmt.Fprintln(os.Stderr, "Error copying from store:", err)
			skipped++
			continue
		}
		pushed++
	}

	fmt.Fprintf(os.Stderr, "Done. Pushed %d files to repo, skipped %d.\n", pushed, skipped)
	return nil
}

func parseOptions(command string, args []string) (syncOptions, error) {
	fsFlags := flag.NewFlagSet(command, flag.ContinueOnError)
	fsFlags.SetOutput(io.Discard)

	var opts syncOptions
	fsFlags.StringVar(&opts.repoHint, "repo", "", "repo path (defaults to current dir repo)")
	fsFlags.IntVar(&opts.worktreeNum, "worktree", 0, "worktree number (1-indexed)")
	fsFlags.StringVar(&opts.destOverride, "dest", "", "destination worktree path")
	fsFlags.BoolVar(&opts.yes, "yes", false, "skip global proceed confirmation")
	fsFlags.BoolVar(&opts.force, "force", false, "overwrite files without per-file prompting")

	if err := fsFlags.Parse(args); err != nil {
		return syncOptions{}, err
	}
	return opts, nil
}

func usageError(command string, err error) error {
	msg := strings.TrimSpace(err.Error())
	if msg != "" {
		fmt.Fprintln(os.Stderr, "Error:", msg)
	}
	fmt.Fprintf(os.Stderr, "usage: wtm %s [--repo PATH] [--worktree N | --dest PATH] [--yes] [--force]\n", command)
	return fmt.Errorf("invalid arguments")
}

func pickWorktree(repoRoot string, wts []gitx.Worktree, destOverride string, worktreeNum int) (gitx.Worktree, error) {
	if destOverride != "" {
		for _, wt := range wts {
			if samePath(wt.Path, destOverride) {
				return wt, nil
			}
		}
		return gitx.Worktree{}, fmt.Errorf("--dest did not match an active worktree path: %s", destOverride)
	}

	if worktreeNum != 0 {
		if worktreeNum < 1 || worktreeNum > len(wts) {
			return gitx.Worktree{}, fmt.Errorf("--worktree must be between 1 and %d", len(wts))
		}
		return wts[worktreeNum-1], nil
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
			return wts[n-1], nil
		}
		fmt.Fprintf(os.Stderr, "Invalid selection. Enter a number between 1 and %d.\n", len(wts))
	}
}

func buildSyncPlan(repoRoot, worktreeRoot, storeRoot string, cfg config.Config) ([]planItem, error) {
	repoRoot = filepath.Clean(repoRoot)
	worktreeRoot = filepath.Clean(worktreeRoot)
	storeRoot = filepath.Clean(storeRoot)

	include := normalizePatterns(cfg.Include)
	exclude := normalizePatterns(cfg.Exclude)

	var items []planItem

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
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
		if !matchesAny(include, rel) || matchesAny(exclude, rel) {
			return nil
		}
		dest := filepath.Join(worktreeRoot, relOS)
		if samePath(path, dest) {
			return nil
		}
		store := filepath.Join(storeRoot, relOS)
		items = append(items, planItem{
			rel:         rel,
			repoAbs:     path,
			storeAbs:    store,
			worktreeAbs: dest,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortPlan(items)
	return items, nil
}

func buildPushPlan(storeRoot, repoRoot string, cfg config.Config) ([]planItem, error) {
	repoRoot = filepath.Clean(repoRoot)
	storeRoot = filepath.Clean(storeRoot)

	include := normalizePatterns(cfg.Include)
	exclude := normalizePatterns(cfg.Exclude)

	var items []planItem

	err := filepath.WalkDir(storeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		relOS, err := filepath.Rel(storeRoot, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		if !matchesAny(include, rel) || matchesAny(exclude, rel) {
			return nil
		}
		repo := filepath.Join(repoRoot, relOS)
		items = append(items, planItem{
			rel:      rel,
			repoAbs:  repo,
			storeAbs: path,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sortPlan(items)
	return items, nil
}

func printSyncPlan(repoRoot, worktreeRoot, storeRoot, configSource string, plan []planItem) {
	fmt.Fprintln(os.Stderr, "Repo:", repoRoot)
	fmt.Fprintln(os.Stderr, "Worktree:", worktreeRoot)
	fmt.Fprintln(os.Stderr, "Store:", storeRoot)
	fmt.Fprintln(os.Stderr, "Config:", configSource)
	fmt.Fprintf(os.Stderr, "Planned entries: %d\n", len(plan))
	for _, it := range plan {
		fmt.Fprintf(os.Stdout, "%s -> %s -> %s\n", it.repoAbs, it.storeAbs, it.worktreeAbs)
	}
}

func printPushPlan(repoRoot, storeRoot, configSource string, plan []planItem) {
	fmt.Fprintln(os.Stderr, "Repo:", repoRoot)
	fmt.Fprintln(os.Stderr, "Store:", storeRoot)
	fmt.Fprintln(os.Stderr, "Config:", configSource)
	fmt.Fprintf(os.Stderr, "Planned entries: %d\n", len(plan))
	for _, it := range plan {
		fmt.Fprintf(os.Stdout, "%s -> %s\n", it.storeAbs, it.repoAbs)
	}
}

func storeRootPath(repoRoot string, worktree gitx.Worktree) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	repoSlug := sanitizeName(filepath.Base(repoRoot))
	if repoSlug == "" {
		repoSlug = "repo"
	}
	segments := worktreePathSegments(repoRoot, worktree)
	if len(segments) == 0 {
		segments = []string{"worktree"}
	}
	parts := append([]string{home, storeRootDir, storeSubDir, repoSlug}, segments...)
	return filepath.Join(parts...), nil
}

func worktreePathSegments(repoRoot string, worktree gitx.Worktree) []string {
	rel, err := filepath.Rel(repoRoot, worktree.Path)
	if err == nil && rel != "" && rel != "." {
		return sanitizeSegments(strings.Split(filepath.ToSlash(rel), "/"))
	}
	return sanitizeSegments(splitPathComponents(worktree.Path))
}

func splitPathComponents(path string) []string {
	if path == "" {
		return nil
	}
	segments := strings.Split(filepath.ToSlash(path), "/")
	o := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" || seg == "." {
			continue
		}
		o = append(o, seg)
	}
	return o
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == filepath.Separator || r == '/' || r == '\\' || r == ' ' || r == ':' || r == '\t':
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "_ ")
}

func sanitizeSegments(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if sanitized := sanitizeName(part); sanitized != "" {
			out = append(out, sanitized)
		}
	}
	return out
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
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && items[j-1].rel > items[j].rel {
			items[j-1], items[j] = items[j], items[j-1]
			j--
		}
	}
}

func copyRepoToStore(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	return copyFileContents(src, dst)
}

func copyStoreToRepo(src, dst string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	if err := handleExisting(dst, force); err != nil {
		return err
	}
	return copyFileContents(src, dst)
}

func handleExisting(path string, force bool) error {
	if _, err := os.Lstat(path); err == nil {
		if !force {
			if !confirm(fmt.Sprintf("Overwrite %s? [y/N] ", path)) {
				return skipError{dst: path}
			}
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return nil
}

func copyFileContents(src, dst string) error {
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
	_ = os.Chtimes(dst, atime, mtime)
	return nil
}

func ensureWorktreeLink(target, link string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(link), err)
	}
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			current, err := os.Readlink(link)
			if err == nil && current == target {
				return nil
			}
		}
		if err := handleExisting(link, force); err != nil {
			return err
		}
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", link, err)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", link, target, err)
	}
	return nil
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

func samePath(a, b string) bool {
	aa := filepath.Clean(a)
	bb := filepath.Clean(b)
	return aa == bb
}
