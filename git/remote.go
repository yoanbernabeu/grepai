package git

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// RepoIdentity names the repository that contains path independently of where
// it is checked out: the normalized URL of its origin remote, followed by
// "#<dir>" when path is a subdirectory of the repository. Clones and linked
// worktrees of one repository get the same identity on every machine. ok is
// false when path is not in a git repository, has no origin remote, or the
// remote is a local path or cannot be normalized. err reports that git could
// not answer (not installed, timed out, or refusing a repository it does not
// trust), so that callers do not mistake it for "no repository".
func RepoIdentity(path string) (identity string, ok bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	run := func(args ...string) (string, bool, error) {
		// #nosec G204 -- fixed git subcommands; path is passed as an argument.
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", path}, args...)...)
		cmd.Env = append(gitFreeEnv(), "LC_ALL=C") // stable messages to match below
		out, err := cmd.Output()
		if err == nil {
			return strings.TrimSpace(string(out)), true, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && ctx.Err() == nil {
			// Recent git says "dubious ownership", older releases with the
			// safe.directory backport "unsafe repository".
			if stderr := string(exitErr.Stderr); strings.Contains(stderr, "dubious ownership") || strings.Contains(stderr, "unsafe repository") {
				return "", false, fmt.Errorf("git refuses %s: %s", path, strings.TrimSpace(string(exitErr.Stderr)))
			}
			return "", false, nil // not a repository, or no origin remote
		}
		return "", false, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	// get-url applies url.<base>.insteadOf rewrites, unlike reading the config.
	remote, found, err := run("remote", "get-url", "origin")
	if err != nil || !found {
		return "", false, err
	}
	identity = NormalizeRemoteURL(remote)
	if identity == "" {
		return "", false, nil
	}
	prefix, found, err := run("rev-parse", "--show-prefix")
	if err != nil || !found {
		return "", false, err
	}
	if dir := strings.Trim(prefix, "/"); dir != "" {
		identity += "#" + dir
	}
	return identity, true, nil
}

// NormalizeRemoteURL reduces a git remote URL to host/path so that the SSH,
// scp-like and HTTPS forms of one repository compare equal:
// git@github.com:org/app.git, ssh://git@github.com/org/app and
// https://github.com/org/app.git all become github.com/org/app. The host is
// lowercased; the scheme, user, password or token, port, query, ".git" suffix
// and surrounding slashes are dropped, so no credential embedded in the URL is
// kept. It returns "" for local path and file:// remotes, which do not name the
// same repository on another machine, and when nothing usable remains.
func NormalizeRemoteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var host, path string
	if i := strings.Index(raw, "://"); i > 0 {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		host, path = u.Hostname(), u.Path
	} else if host, path = scpLike(raw); host == "" {
		// A local path (absolute or relative to the repository) does not
		// name the same repository on another machine.
		return ""
	}
	path = strings.Trim(path, "/")
	path = strings.Trim(strings.TrimSuffix(path, ".git"), "/")
	if path == "" {
		return ""
	}
	if host == "" {
		return "" // file:// URL: a local path
	}
	return strings.ToLower(host) + "/" + path
}

// scpLike splits git's scp-like syntax, [user@]host:path, dropping the user
// (and anything credential-like before the last "@" of the host part). It
// returns an empty host when raw is not in that form, e.g. a local path or a
// Windows drive letter.
func scpLike(raw string) (host, path string) {
	hostPart := raw
	if slash := strings.IndexAny(raw, `/\`); slash >= 0 {
		hostPart = raw[:slash]
	}
	rest := raw
	if at := strings.LastIndex(hostPart, "@"); at >= 0 {
		rest = raw[at+1:]
	}
	i := strings.Index(rest, ":")
	if i <= 1 || strings.ContainsAny(rest[:i], `/\`) {
		return "", ""
	}
	return rest[:i], rest[i+1:]
}

// gitFreeEnv is the environment without the variables git sets for hooks
// (GIT_DIR, GIT_WORK_TREE, GIT_PREFIX, ...), which would make "git -C path"
// answer for another repository or prefix than path when grepai runs from a
// hook.
func gitFreeEnv() []string {
	env := os.Environ()
	kept := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_PREFIX", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM":
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}
