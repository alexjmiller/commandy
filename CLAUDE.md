# Commandy

Terminal session launcher that provides quick access to common development tasks. Launches automatically when opening a new terminal.

## Overview

Commandy displays a menu on terminal startup with host-specific options:

**On `mac` (Mac Mini):**
1. BlueStudio management
2. Browse Projects
3. Setup New Project
4. Tools
5. Skip

**On other machines (MacBookPro):**
1. Fintrac management
2. Browse Projects
3. Setup New Project
4. Tools
5. Skip

## Host Detection

Uses `$(hostname)` to determine which machine-specific options to show:
- `mac` - Shows BlueStudio option
- Other - Shows Fintrac option

## Tools Menu

The Tools menu provides access to utility submenus:

### Quick Access
- **SSH** - Connect to mac (from MacBookPro) or MacBookPro (from mac)
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

## Service Management Menus

### BlueStudio (mac only)
1. Start (both services)
2. Stop all services
3. Restart all services
4. Check status
5. View API logs
6. View Dashboard logs

### Fintrac (MacBookPro only)
1. Start (all services) - Docker + Backend + Frontend
2. Stop all services
3. Restart all services
4. Check status
5. View Backend logs
6. View Frontend logs

## Browse Projects
Lists all directories in `~/Projects` and allows:
- Opening the folder (cd + new shell)
- Launching claude-logged in that project

## Setup New Project
- Creates new directory in `~/Projects`
- Initializes git repository
- Options to start working or launch claude-logged

## Installation

The script is set up globally via `.zshrc`:

```bash
# In ~/.zshrc
export PATH="$HOME/Projects/commandy:$PATH"
if [[ $- == *i* ]]; then
    commandy
fi
```

## Dependencies

### External Commands
- `bluestudio` - BlueStudio service manager (on mac)
- `~/Projects/fintrac/fintrac` - Fintrac service manager (on MacBookPro)
- `claude-logged` - Claude wrapper with logging
- `ngrok` - For tunnel/webhook testing
- `docker` - For Docker cleanup and Fintrac services
- `brew` - Homebrew package manager
- `psql` - PostgreSQL client

### Related Projects
- **fintrac** (`~/Projects/fintrac/fintrac`) - Local dev service manager
  - Manages: PostgreSQL, Redis (Docker), Backend (Express), Frontend (Vite)
  - Commands: start, stop, restart, status, logs

## Ports Reference

Common ports checked by Dev Tools:
- 3000 - Generic dev server
- 3012 - Fintrac Backend API
- 5173 - Fintrac Frontend (Vite)
- 5432 - PostgreSQL
- 6379 - Redis
- 8080 - Generic web server
