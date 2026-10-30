package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeRemoteURL(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"git@github.com:org/app.git", "github.com/org/app"},
		{"ssh://git@github.com/org/app", "github.com/org/app"},
		{"https://github.com/org/app.git", "github.com/org/app"},
		{"https://github.com/org/app", "github.com/org/app"},
		{"https://github.com/org/app/", "github.com/org/app"},
		{"  https://GitHub.COM/org/app.git\n", "github.com/org/app"},
		{"ssh://git@gitlab.example.com:2222/group/sub/app.git", "gitlab.example.com/group/sub/app"},
		{"https://user:s3cr3t-token@github.com/org/app.git", "github.com/org/app"},
		{"https://oauth2:glpat-xyz@gitlab.com/org/app.git?private_token=abc#frag", "gitlab.com/org/app"},
		{"deploy:token@github.com:org/app.git", "github.com/org/app"},
		{"git://github.com/org/app.git", "github.com/org/app"},
		{"file:///srv/git/app.git", ""},
		{"/srv/git/app.git", ""},
		{"../app.git", ""},
		{`C:\repos\app.git`, ""},
		{"", ""},
		{"https://github.com/", ""},
	} {
		if got := NormalizeRemoteURL(tc.raw); got != tc.want {
			t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestRepoIdentity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	noRemote := t.TempDir()
	run(noRemote, "init", "-q")
	if id, ok, err := RepoIdentity(noRemote); ok || err != nil {
		t.Fatalf("repository without origin got identity %q, err %v", id, err)
	}
	if id, ok, err := RepoIdentity(t.TempDir()); ok || err != nil {
		t.Fatalf("directory outside git got identity %q, err %v", id, err)
	}

	a, b := t.TempDir(), t.TempDir()
	run(a, "init", "-q")
	run(a, "remote", "add", "origin", "git@github.com:org/app.git")
	run(b, "init", "-q")
	run(b, "remote", "add", "origin", "https://user:token@github.com/org/app")
	idA, okA, errA := RepoIdentity(a)
	idB, okB, errB := RepoIdentity(b)
	if errA != nil || errB != nil {
		t.Fatal(errA, errB)
	}
	if !okA || !okB || idA != "github.com/org/app" || idB != idA {
		t.Fatalf("clones got identities %q (%v) and %q (%v), want github.com/org/app", idA, okA, idB, okB)
	}
	sub := filepath.Join(a, "services", "api")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if id, ok, err := RepoIdentity(sub); !ok || err != nil || id != "github.com/org/app#services/api" {
		t.Fatalf("subdirectory identity = %q (%v, %v)", id, ok, err)
	}
	// url.<base>.insteadOf rewrites apply, as for git itself.
	c := t.TempDir()
	run(c, "init", "-q")
	run(c, "config", "url.git@github.com:.insteadOf", "gh:")
	run(c, "remote", "add", "origin", "gh:org/app.git")
	if id, ok, err := RepoIdentity(c); !ok || err != nil || id != "github.com/org/app" {
		t.Fatalf("insteadOf identity = %q (%v, %v)", id, ok, err)
	}
	// A local path remote does not identify the repository.
	d := t.TempDir()
	run(d, "init", "-q")
	run(d, "remote", "add", "origin", "../app.git")
	if id, ok, err := RepoIdentity(d); ok || err != nil {
		t.Fatalf("local remote got identity %q, err %v", id, err)
	}
	// Run from a git hook of another repository, the identity is unchanged.
	t.Setenv("GIT_DIR", filepath.Join(b, ".git"))
	if id, ok, err := RepoIdentity(sub); !ok || err != nil || id != "github.com/org/app#services/api" {
		t.Fatalf("subdirectory identity with GIT_DIR set = %q (%v, %v)", id, ok, err)
	}
}
