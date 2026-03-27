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
6. Skip

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
- `dev.lan` — Hides "Connect to dev", shows Sessions (tmux)
- `mac` — Hides "Connect to mac"
- Other — Shows both Connect options, no Sessions

## Implementation

- **Active codebase:** `main.go` (Go + Bubble Tea TUI)
- **Legacy:** `commandy.sh` (shell script, no longer the primary version)
- **Binary:** `commandy` (compiled Go binary, must be rebuilt with `go build` after changes)
- **Assets:** `commandy.png`, `commandy2.png` (banner images rendered via chafa)

## Tmux Integration

On machines with tmux available:
- **Browse Projects** opens projects in named tmux sessions
- **Sessions** menu lists active tmux sessions with attach/kill options
- **Claude-logged** launches in a tmux session and returns to commandy when the session exits

Without tmux, commands fall back to direct shell execution.

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
- Launching claude-logged in that project (returns to commandy on exit) — default first option
- Opening the folder in a tmux session (or new shell without tmux)
- If an existing tmux session exists for the project, "Attach" becomes the first option above claude-logged

## Setup New Project
- Creates new directory in `~/Projects`
- Initializes git repository
- Options to start working or launch claude-logged

## Installation

The binary is set up globally via `.zshrc`:

```bash
# In ~/.zshrc
export PATH="$HOME/Projects/commandy:$PATH"
if [[ $- == *i* ]]; then
    commandy
fi
```

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
- `claude-logged` - Claude wrapper with logging
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
