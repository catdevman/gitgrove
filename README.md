# GitGrove 🌳

A happy little tool for grouping git repos into task-scoped workspaces — so your AI coding assistant can see exactly what it needs and nothing it doesn't.

## The Problem

You're working on a feature that touches three repos. Your AI tool needs a single directory to work from. So you either point it at some massive parent folder full of unrelated projects, or you start copying repos around like it's 2003.

Neither of these spark joy.

## The Solution

GitGrove uses **git worktrees** to link repos into a shared directory called a **grove** — no copying, no duplication, just clean linked checkouts that share the same git history. Point your AI at the grove, and it gets exactly the context it needs.

```
~/groves/
└── my-feature/
    ├── backend/      ← worktree of ~/code/backend @ feat/new-api
    ├── frontend/     ← worktree of ~/code/frontend @ feat/new-ui
    ├── shared-lib/   ← worktree of ~/code/shared-lib @ main
    └── api-docs/     ← copy of ~/notes/api-docs (not a repo)
```

One directory. Three repos. Zero copies — plus whatever plain directories you
want along for the ride.

## Install

```sh
go install github.com/catdevman/gitgrove@latest
```

## Quick Start

```sh
# Plant a grove
gitgrove create my-feature

# Add repos — local paths or remote URLs, branch is after the colon
gitgrove add my-feature ~/code/backend:feat/new-api
gitgrove add my-feature https://github.com/org/frontend:feat/new-ui

# Add a plain directory — no branch, no repo needed
gitgrove add my-feature ~/notes/api-docs

# Validate your config before doing anything
gitgrove doctor my-feature

# Grow the worktrees (clones remotes automatically on first sync)
gitgrove sync my-feature

# Point your AI at ~/groves/my-feature and get to work
```

## Commands

| Command | What it does |
|---|---|
| `gitgrove create <name>` | Create a new grove |
| `gitgrove delete <name>` | Remove a grove from config (`--prune` to also remove from disk) |
| `gitgrove list` | See all your groves |
| `gitgrove add <grove> <source:branch> ...` | Add one or more repos to a grove |
| `gitgrove add <grove> <dir> ...` | Add one or more plain directories (no branch) |
| `gitgrove remove <grove> <name>` | Remove a repo or directory from a grove |
| `gitgrove sync [grove]` | Create missing worktrees and directories, and fix any drift |
| `gitgrove status [grove]` | Check what's synced, what's missing, what's drifted |
| `gitgrove doctor [grove]` | Validate config before you sync — exits non-zero on errors |

## Status at a Glance

```sh
$ gitgrove status my-feature

GROVE       REPO      STATUS          CONFIG BRANCH  ACTUAL BRANCH
my-feature  backend   OK              feat/new-api   feat/new-api
my-feature  frontend  BRANCH MISMATCH feat/new-ui    main
my-feature  shared    MISSING         main
```

Run `gitgrove sync my-feature` and it will fix the branch mismatch and create the missing worktree in one pass.

## Doctor

Run `doctor` before `sync` to catch problems early:

```sh
$ gitgrove doctor my-feature

GROVE       REPO      CHECK       STATUS  DETAIL
my-feature  backend   source      OK      ~/code/backend
my-feature  backend   branch      OK      feat/new-api
my-feature  frontend  source      OK      remote URL (will clone to ~/.local/share/gitgrove/repos/github.com/org/frontend on sync)
my-feature  shared    source      ERROR   not a git repo: ~/code/typo
```

Exits with a non-zero status if any errors are found — useful in scripts or CI.

## Plain Directories

Not everything you want in a grove is a repo. Reference docs, a code example,
a vendored SDK — anything with no git history behind it has no worktree to
create, so GitGrove copies it in instead. Leave the branch off and it is
treated as a plain directory:

```sh
gitgrove add my-feature ~/notes/api-docs
gitgrove add my-feature ~/notes/api-docs --name docs   # rename it in the grove
```

**Copies are the default** because a grove is disposable: the copy is pruned
along with everything else, and nothing that happens inside the grove — an
agent rewriting files, say — can reach the original directory.

When you do want the grove to point at the live directory, so edits land in the
source:

```sh
gitgrove add my-feature ~/code/some-example --symlink
```

A few details:

- **Copies are made once.** Re-running `sync` never overwrites a copy, so work
  done inside the grove is safe.
- **Symlinks are kept pointing at the source.** If you change the source in the
  config, `sync` retargets the symlink — the same way it fixes branch drift.
- **Adding a git repo without a branch is an error**, since copying a whole
  repo including `.git` is rarely what was meant. Pass `--dir` if it is.
- **Pruning a symlink never touches the source** — only the link is removed.

```sh
$ gitgrove status my-feature

GROVE       REPO      STATUS  CONFIG BRANCH  ACTUAL BRANCH
my-feature  backend   OK      feat/new-api   feat/new-api

GROVE       DIR       STATUS   MODE     SOURCE
my-feature  api-docs  OK       copy     ~/notes/api-docs
my-feature  example   MISSING  symlink  ~/code/some-example
```

## Remote Repos

You can add repos by URL — GitGrove will clone them automatically on first sync:

```sh
gitgrove add my-feature https://github.com/org/repo:feat/thing
gitgrove add my-feature git@github.com:org/repo.git:main
```

Clones are cached under `~/.local/share/gitgrove/repos/` using a directory structure that mirrors the URL:

```
~/.local/share/gitgrove/repos/
└── github.com/
    └── org/
        └── repo/     ← bare clone, shared across all groves
```

Subsequent syncs reuse the cached clone — no re-downloading.

## Config

Everything lives in `~/.config/gitgrove/config.toml`. It's the source of truth — `sync` reconciles the filesystem to match it.

```toml
# Optional: override where remote repos are cached (default: ~/.local/share/gitgrove/repos)
cache_dir = "~/code"

[groves.my-feature]
path = "~/groves/my-feature"

[[groves.my-feature.repos]]
name = "backend"
source = "~/code/backend"
branch = "feat/new-api"

[[groves.my-feature.repos]]
name = "frontend"
source = "https://github.com/org/frontend"
branch = "feat/new-ui"

[[groves.my-feature.dirs]]
name = "api-docs"
source = "~/notes/api-docs"
mode = "copy"   # or "symlink"; defaults to copy
```

Edit it by hand or use the CLI — either works.

## A Few Things to Know

- **Groves live at `~/groves/<name>` by default.** Override with `--path` on `create`.
- **Branches are created automatically.** If the branch you specify doesn't exist yet, `sync` creates it from HEAD.
- **`sync` fixes branch drift.** If a worktree exists but is on the wrong branch, `sync` switches it — no need to remove and re-add.
- **Remote repos are cloned on demand.** Pass an `https://` or `git@` URL as the source and `sync` handles the clone on first use.
- **Nothing is duplicated.** Worktrees share the same git object store as your source repo. Disk usage is minimal. (Plain directories are the exception — those are copied, unless you ask for `--symlink`.)
- **`sync` is safe to re-run.** It skips anything already in the correct state.

## Why Worktrees?

Git worktrees let you check out a branch into a new directory without cloning the repo again. The `.git` data lives in one place; the working files can live anywhere. GitGrove just does this for multiple repos at once and keeps track of the whole picture for you.

---

Happy little trees. 🌱
