package gitx

import "testing"

func TestParseWorktreePorcelain(t *testing.T) {
	in := `
worktree /repo
HEAD 1111111111111111111111111111111111111111
branch refs/heads/develop

worktree /repo-wt
HEAD 2222222222222222222222222222222222222222
branch refs/heads/feat/x
`

	wts, err := parseWorktreePorcelain(in)
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2, got %d", len(wts))
	}
	if wts[0].Path != "/repo" || wts[0].Branch != "refs/heads/develop" || wts[0].Head[:7] != "1111111" {
		t.Fatalf("unexpected first: %#v", wts[0])
	}
	if wts[1].Path != "/repo-wt" || wts[1].Branch != "refs/heads/feat/x" || wts[1].Head[:7] != "2222222" {
		t.Fatalf("unexpected second: %#v", wts[1])
	}
}

