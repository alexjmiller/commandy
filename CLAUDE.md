# Commandy

Terminal session launcher that provides quick access to common development tasks. Launches automatically when opening a new terminal. Built with Go using the Bubble Tea TUI framework.

## Overview

Commandy displays a menu on terminal startup with host-specific options:

**On `dev.lan` (Mac Mini):**
1. Browse Projects
2. Setup New Project
3. Tools
4. Connect to mac
5. Sessions (tmux session manager)
6. Help (copy/paste keystroke guide)
7. Exit

**On `mac` (MacBook):**
1. Connect to dev
2. Browse Projects
3. Setup New Project
4. Tools
5. Skip

**On other machines:**
1. Connect to dev
2. Connect to mac
3. Browse Projects
4. Setup New Project
5. Tools
6. Skip

## Host Detection

Uses `os.Hostname()` to determine which machine-specific options to show:
- `dev.lan` (also matches `dev.local`, `dev`) — Hides "Connect to dev", shows Sessions (tmux)
- `mac` — Hides "Connect to mac"
- Other — Shows both Connect options, no Sessions

## Implementation

- **Active codebase:** `main.go` (Go + Bubble Tea TUI)
- **Legacy:** `commandy.sh` (shell script, no longer the primary version)
- **Binary:** `commandy` (compiled Go binary, must be rebuilt with `go build` after changes)
- **Assets:** `commandy.png`, `commandy2.png` (banner images rendered via chafa)

## Tmux Integration

On machines with tmux available:
- **Browse Projects** opens projects in named tmux sessions (`tmux new -s <project-name>`)
- **Sessions** menu lists active tmux sessions (sorted) with attach/kill options. Killing a session returns to the list. Commandy refuses to kill the session it is itself running in (the title shows which one), since that would kill commandy's own pane — detach first and kill from outside.
- **Claude-logged** launches Claude in a separate tmux session. Commandy stays alive in the background using `tmux wait-for` to block until Claude finishes. When the Claude session ends (signalling via `tmux wait-for -S done-<session>`), the tmux session auto-closes and commandy returns to the main menu — no stale sessions left behind.

Without tmux, commands fall back to direct shell execution.

## Session Logging

When launching Claude from commandy (via "Claude-logged" menu option), sessions are logged to the claude-logger API:

1. Claude Code runs normally and writes structured JSONL session logs to `~/.claude/projects/`
2. When Claude exits, commandy finds the most recent JSONL file for the project and uploads it to the API via `curl`
3. The API stores the JSONL and serves parsed conversations (user messages, tool calls, responses) to the frontend

No additional dependencies are needed — `curl` is the only external tool used for the upload.

**Limitation:** Logging is post-session only — the JSONL is uploaded after claude exits, not in real time. A future improvement could use a background process to watch the JSONL file and periodically upload new entries during the session, enabling live session viewing in the frontend.

**Environment:**
- `CLAUDE_LOGGER_API` — API URL (defaults to `http://zynx.lan:3000`)

## Tools Menu

The Tools menu provides access to utility submenus:

### Quick Access
- **SSH** - Connect to mac (from dev.lan) or dev (from mac)
- **Open GitHub** - Opens github.com in browser
- **Prisma Studio** - Launch Prisma Studio for any project with package.json
- **PostgreSQL shell** - Connect to psql (defaults to Fintrac database)

### Dev Tools
- **Kill process on port** - Find and kill processes using a specific port
- **Check port usage** - Show what's using common ports (3000, 3012, 5173, 5432, 6379, 8080)
- **Start ngrok** - Expose a local port (default 3012) for webhook testing
- **Git status (all projects)** - Quick overview of all repos showing branch and dirty state
- **Git pull (all projects)** - Pull latest changes for all git repositories

### Port Authority
Centralized port management across the local network via Port Authority service.

- **Check project ports** - View registered ports for a project, offer to set up if none exist
- **Setup ports for project** - Register a new port with auto-suggested available port
- **Update project port** - Change port number, description, or delete registration
- **View all registered ports** - Table view of all ports across all projects/hosts
- **Open dashboard** - Launch Port Authority web dashboard

**API:** http://zynx.lan:3030/api
**Dashboard:** http://zynx.lan:8000

### System Maintenance
- **Docker cleanup** - Remove stopped containers, unused images, volumes, and networks
- **Homebrew update** - Update, upgrade, and cleanup Homebrew packages
- **Clear npm cache** - Force clean npm cache
- **Remove node_modules** - Delete node_modules from selected project
- **Clear all caches** - npm cache + Homebrew cache + .DS_Store files

### NPM Utilities
- **npm audit** - Run security audit on selected project
- **npm outdated** - Check for outdated packages in selected project
- **npm update** - Update packages in selected project
- **npm dedupe** - Deduplicate packages in selected project
- **npm install** - Install packages in selected project
- **Check outdated (all)** - Show outdated packages across all projects (including monorepo subdirs)

## Project Selection

When selecting a project for npm operations or Prisma Studio:
- Lists all projects with `package.json` in ~/Projects
- Also lists subdirectories with `package.json` (for monorepos like fintrac/backend, fintrac/frontend)

## Browse Projects
Lists all directories in `~/Projects` and allows:
- Launching Claude in that project (returns to commandy on exit) — default first option
- Opening the folder in a tmux session (or new shell without tmux)
- If an existing tmux session exists for the project, "Attach" becomes the first option above Claude
- **New shell window** (only with an existing session) — opens a plain shell in a new window of that session, in the project folder, and attaches; for running a dev server or other commands alongside a Claude session (switch windows with Ctrl+A n / p)
- **Git** — opens a submenu that adapts to the folder (`loadGitInfo`): a summary at the top shows last commit (relative + absolute date, subject), branch with ahead/behind vs upstream, uncommitted file count, and remote URL. Items: `Git init` only when the folder is not a repo; otherwise `Git status`, `Git log` (last 10), and `Git pull` / `Git push` only when a remote exists (push uses `-u origin <branch>` if there is no upstream yet). Commands run in the background and show git's own output; nothing is ever stashed, reset, or force-pushed, so a dirty tree makes pull fail with git's message. The summary refreshes after each command.

## Menu Number Entry
Menus accept multi-digit numbers. Typing a digit moves the cursor; if another digit could still form a valid item number (e.g. `1` with 12+ items) it waits ~700ms for it, showing `number: 1_`. A second digit, Enter, or the timeout selects. Out-of-range entries are discarded. The trailing "Back" item is pinned (`pinnedBack`) to **9** in short menus and **99** in menus with 10+ items, so it sits on the same key everywhere regardless of item count; in long menus `9` waits for a second digit. The main menu has no Back and Exit keeps its natural number, so a repeated `9` never exits by accident.

## Help
Root-menu entry showing a keystroke guide for copy/paste inside tmux (`renderCopyPasteHelp`). Copies inside tmux go to the tmux buffer, dev.lan's clipboard, and the connecting Mac's clipboard via `~/.local/bin/tmux-clip` (configured in `~/.tmux.conf`, because Apple Terminal.app ignores OSC 52). Keep the guide in sync with `~/.tmux.conf` if bindings change.

## Setup New Project
- Creates new directory in `~/Projects`
- Initializes git repository
- Options to start working or launch Claude

## Installation

The binary is set up globally via `.zshrc`:

```bash
# In ~/.zshrc
export CLAUDE_LOGGER_API="http://zynx.lan:3000"
export PATH="$HOME/Projects/commandy:$PATH"
if [[ $- == *i* && -z "$TMUX" ]]; then
    commandy
fi
```

The `-z "$TMUX"` guard matters: tmux sessions that commandy opens ("Start working here", "Open") run an interactive zsh, which would otherwise relaunch commandy and show the main menu inside the new session instead of a prompt.

### Building

```bash
go build -o commandy .
```

### Cross-compiling

```bash
GOOS=darwin GOARCH=amd64 go build -o commandy   # Intel Mac
GOOS=linux GOARCH=amd64 go build -o commandy     # Linux x86_64
```

## Dependencies

### Go Modules
- `github.com/charmbracelet/bubbletea` - TUI framework
- `github.com/charmbracelet/bubbles` - TUI components (text input)
- `github.com/charmbracelet/lipgloss` - TUI styling

### External Commands
- `tmux` - Terminal multiplexer (optional, enables Sessions and tmux-based project management)
- `chafa` - Terminal image renderer (optional, for banner logo)
- `claude` - Claude Code CLI
- `curl` - HTTP client for uploading session logs to API (built-in)
- `ngrok` - For tunnel/webhook testing
- `docker` - For Docker cleanup
- `brew` - Homebrew package manager
- `psql` - PostgreSQL client

## Ports Reference

Common ports checked by Dev Tools:
- 3000 - Generic dev server
- 3012 - Fintrac Backend API
- 5173 - Fintrac Frontend (Vite)
- 5432 - PostgreSQL
- 6379 - Redis
- 8080 - Generic web server
