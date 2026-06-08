# Fork-Based Rig Setup

When you run a rig against a repository you **don't own**, the rig has to
fetch canonical history from upstream but push Gas Town-managed work to your
fork.
`gt rig add` supports this directly through `--push-url` and
`--upstream-url`. Without them, the default `gt rig add <name> <fork-url>`
produces a rig whose refinery merges polecat work into your fork's `main`,
diverging it from upstream.

## When You Need Fork Mode

Use fork mode whenever:

- You have read-only access to the canonical repo (e.g. you're an external
  contributor), and
- You push your work to a personal/organization fork and open PRs from there.

If you own the canonical repo and push directly to it, you do **not** need
these flags — the plain `gt rig add <name> <git-url>` is correct.

## Setup

```bash
gt rig add <name> <upstream-url> \
  --push-url     <your-fork-url> \
  --upstream-url <upstream-url>
```

Concretely, for a Gas Town contributor:

```bash
gt rig add gastown https://github.com/gastownhall/gastown \
  --push-url     https://github.com/<you>/gastown \
  --upstream-url https://github.com/gastownhall/gastown
```

What each flag does:

| Flag | Effect |
|---|---|
| positional `<git-url>` | `origin`'s **fetch** URL — where canonical history is pulled from |
| `--push-url` | `origin`'s **push** URL — where Gas Town-managed pushes and `git push origin ...` go (your fork) |
| `--upstream-url` | Adds a separate named `upstream` remote for fetching, comparing, and rebasing against canonical history |

These remotes are configured on the shared bare repository
(`<town>/<rig>/.repo.git`) and the mayor's working clone
(`<town>/<rig>/mayor/rig`). The refinery directory
(`<town>/<rig>/refinery/rig`) is a worktree backed by `.repo.git`, so its
remote configuration comes from the shared bare repo.

## Verifying the Setup

Check the remotes in the shared bare repo, the refinery worktree, and the
mayor's clone:

```bash
git --git-dir <town>/<rig>/.repo.git remote -v
cd <town>/<rig>/refinery/rig && git remote -v
cd <town>/<rig>/mayor/rig && git remote -v
```

Expect (substituting your fork and the canonical repo):

```
origin    https://github.com/gastownhall/gastown (fetch)
origin    https://github.com/<you>/gastown       (push)
upstream  https://github.com/gastownhall/gastown (fetch)
upstream  https://github.com/gastownhall/gastown (push)
```

The `upstream ... (push)` line is Git's default effective push URL for that
remote. In this workflow, `upstream` is for fetch/comparison/rebase operations;
do not run `git push upstream ...` unless you are a maintainer intentionally
updating the canonical repo.

The key invariant: **`origin`'s fetch URL is upstream, `origin`'s push URL
is your fork.** If `origin (push)` points at the canonical repo, the flags
did not take effect — update or re-add the rig before running agents.

## Current Limitation: Runtime Fork Workflows Are Not Yet Enforced

Even a correctly-configured fork rig will, today, have its refinery attempt
to **merge polecat branches into the fork's `main`** rather than open a PR
to upstream. The foundation flags (`--push-url` / `--upstream-url`) shipped
in [gastownhall/gastown#2018](https://github.com/gastownhall/gastown/pull/2018),
but runtime safeguards and upstream-PR behavior are still tracked in
[gastownhall/gastown#4045](https://github.com/gastownhall/gastown/issues/4045).
[#1794](https://github.com/gastownhall/gastown/issues/1794) is the original
historical issue for the PR-to-upstream workflow.

Until then, for strict PR-only behavior:

- Do **not** start or keep running the refinery. `gt rig park <rig>` stops the
  whole rig if you need a durable stop.
- Push feature branches to your fork and open PRs to upstream manually. The
  existing
  [polecat PR-flow harness](../contrib-harnesses/polecat-pr-flow/README.md) is
  the closest reference for that branch-to-PR path.

## Recovery: A Polluted Fork `main`

If you added a rig **without** the fork-routing flags, the refinery may
have already merged polecat work into your fork's `main`, leaving it with
mixed `Merge branch ...` and refinery-generated commits diverged from
upstream.

> **Destructive — consult a maintainer before running.** Resetting `main`
> rewrites your fork's history. If any of the diverged commits contain work
> you still need (unmerged PRs, local-only fixes), stop and recover those
> branches first. A backup branch is the primary escape hatch; `git reflog`
> is local and time-limited.

The commands below assume the simple bad setup where `origin` fetches your fork,
as it does after `gt rig add <name> <fork-url>` with no fork-routing flags. If
`origin` already fetches upstream, add a temporary `fork` remote for your fork
and substitute `fork/main` anywhere this recipe uses `origin/main`.

1. Inspect the divergence before touching anything:

   ```bash
   cd <town>/<rig>/mayor/rig
   git status --short
   git remote -v
   git remote get-url upstream >/dev/null 2>&1 || git remote add upstream <upstream-url>
   git fetch origin
   git fetch upstream
   git branch backup/fork-main-before-reset-$(date +%Y%m%d-%H%M%S) origin/main
   git log --oneline --graph upstream/main...origin/main
   ```

   Make sure `git status --short` prints nothing before continuing.

2. Confirm every commit on `origin/main` that is *not* on `upstream/main`
   is safe to discard (it's refinery merge noise, not real work). Salvage
   anything you need onto a separate branch first.

3. Reset `main` to track upstream and force-publish to your fork:

   ```bash
   git checkout main
   git reset --hard upstream/main
   git push --force-with-lease origin main
   ```

4. Re-add the rig **with** the fork-routing flags (see [Setup](#setup)) so
   this doesn't recur.

## See also

- [CONTRIBUTING.md](../../CONTRIBUTING.md) — "Setting up a rig to contribute
  to Gas Town" (Gas Town-specific worked example)
- [Local Rig Bootstrap](local-rig-bootstrap.md) — local/private repo setup
