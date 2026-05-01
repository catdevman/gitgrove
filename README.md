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
    └── shared-lib/   ← worktree of ~/code/shared-lib @ main
```

One directory. Three repos. Zero copies.

## Install

```sh
go install github.com/catdevman/gitgrove@latest
```

## Quick Start

```sh
# Plant a grove
gitgrove create my-feature

# Add some repos (source:branch pairs)
gitgrove add my-feature ~/code/backend:feat/new-api ~/code/frontend:feat/new-ui

# Grow the worktrees
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
| `gitgrove remove <grove> <name>` | Remove a repo from a grove |
| `gitgrove sync [grove]` | Create any missing worktrees on disk |
| `gitgrove status [grove]` | Check what's synced, what's missing, what's drifted |

## Status at a Glance

```sh
$ gitgrove status my-feature

GROVE       REPO      STATUS   CONFIG BRANCH  ACTUAL BRANCH
my-feature  backend   OK       feat/new-api   feat/new-api
my-feature  frontend  MISSING  feat/new-ui

1 repo in "my-feature" not synced — run: gitgrove sync my-feature
```

No mystery. No digging. Just run the command it tells you.

## Config

Everything lives in `~/.config/gitgrove/config.toml`. It's the source of truth — `sync` reconciles the filesystem to match it.

```toml
[groves.my-feature]
path = "~/groves/my-feature"

[[groves.my-feature.repos]]
name = "backend"
source = "~/code/backend"
branch = "feat/new-api"

[[groves.my-feature.repos]]
name = "frontend"
source = "~/code/frontend"
branch = "feat/new-ui"
```

Edit it by hand or use the CLI — either works.

## A Few Things to Know

- **Groves live at `~/groves/<name>` by default.** Override with `--path` on `create`.
- **Repos must already be cloned locally.** GitGrove links them; it doesn't clone them (yet).
- **Nothing is duplicated.** Worktrees share the same git object store as your source repo. Disk usage is minimal.
- **`sync` is safe to re-run.** It skips repos that are already linked.

## Why Worktrees?

Git worktrees let you check out a branch into a new directory without cloning the repo again. The `.git` data lives in one place; the working files can live anywhere. GitGrove just does this for multiple repos at once and keeps track of the whole picture for you.

---

Happy little trees. 🌱
