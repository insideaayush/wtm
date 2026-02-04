package gitx

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

func RepoRoot(repoHint string) (string, error) {
	args := []string{"rev-parse", "--show-toplevel"}
	cmd := exec.Command("git", args...)
	if repoHint != "" {
		cmd.Args = append([]string{"git", "-C", repoHint}, args...)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find git repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

type Worktree struct {
	Path   string
	Branch string // e.g. "refs/heads/develop" (may be empty)
	Head   string // full sha
}

func ListWorktrees(repoRoot string) ([]Worktree, error) {
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "list", "--porcelain")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("git worktree list failed: %s", msg)
		}
		return nil, fmt.Errorf("git worktree list failed: %w", err)
	}
	return parseWorktreePorcelain(string(out))
}

func parseWorktreePorcelain(s string) ([]Worktree, error) {
	var out []Worktree
	var cur *Worktree

	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "worktree "):
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &Worktree{Path: strings.TrimSpace(strings.TrimPrefix(line, "worktree "))}
		case strings.HasPrefix(line, "branch "):
			if cur != nil {
				cur.Branch = strings.TrimSpace(strings.TrimPrefix(line, "branch "))
			}
		case strings.HasPrefix(line, "HEAD "):
			if cur != nil {
				cur.Head = strings.TrimSpace(strings.TrimPrefix(line, "HEAD "))
			}
		default:
			// ignore other lines like "locked"
		}
	}

	if cur != nil {
		out = append(out, *cur)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no worktrees found")
	}
	return out, nil
}

