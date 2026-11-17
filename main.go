package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Menu states
type menuState int

const (
	stateMain menuState = iota
	stateBrowseProjects
	stateProjectActions
	stateSetupProject
	stateSetupProjectConfirm
	stateTools
	stateQuickAccess
	stateDevTools
	statePortAuthority
	stateSystemMaintenance
	stateNpmUtilities
	stateSessions
	stateSessionActions
	stateSelectProject
	stateInputPort
	stateInputProjectName
	stateInputDbUrl
	stateRunningCommand
	stateHelp
	stateGitMenu
	stateQuit
)

// Project selection context
type projectAction int

const (
	actionPrismaStudio projectAction = iota
	actionRemoveNodeModules
	actionNpmAudit
	actionNpmOutdated
	actionNpmUpdate
	actionNpmDedupe
	actionNpmInstall
)

var (
	projectsDir  = filepath.Join(os.Getenv("HOME"), "Projects")
	hostname     string
	execPath     string // Path to the commandy executable directory
	execFullPath string // Full path to the commandy executable
	cachedBanner string // Cached banner output to avoid re-running chafa on every render

	// Port Authority config
	portAuthorityAPI       = "http://zynx.lan:3030/api"
	portAuthorityDashboard = "http://zynx.lan:8000"
)

// Styles
var (
	cyan    = lipgloss.Color("6")
	blue    = lipgloss.Color("4")
	green   = lipgloss.Color("2")
	yellow  = lipgloss.Color("3")
	red     = lipgloss.Color("1")
	white   = lipgloss.Color("15")
	magenta = lipgloss.Color("5")

	titleStyle = lipgloss.NewStyle().
			Foreground(cyan).
			Bold(true)

	menuBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(cyan).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(green).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(white)

	cursorStyle = lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Foreground(blue).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(yellow)

	errorStyle = lipgloss.NewStyle().
			Foreground(red)

	successStyle = lipgloss.NewStyle().
			Foreground(green)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	bannerBorderStyle = lipgloss.NewStyle().
				Foreground(cyan)

	bannerTextStyle = lipgloss.NewStyle().
			Foreground(blue).
			Bold(true)
)

// isKitty checks if we're running in Kitty terminal
func isKitty() bool {
	return os.Getenv("KITTY_WINDOW_ID") != "" || strings.Contains(os.Getenv("TERM"), "kitty")
}

// renderKittyImage renders an image using Kitty's graphics protocol
func renderKittyImage(imagePath string, cols int) string {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(data)

	// Kitty graphics protocol
	// a=T - transmit and display
	// f=100 - PNG format
	// c=cols - width in columns
	// q=2 - suppress response
	// i=1 - image ID (allows replacement on re-render)
	var sb strings.Builder

	// Delete any existing image with this ID first
	sb.WriteString("\033_Ga=d,d=I,i=1,q=2;\033\\")

	// Split into chunks of 4096 bytes (Kitty protocol requirement)
	chunkSize := 4096
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]

		if i == 0 {
			// First chunk: include all parameters
			if end >= len(encoded) {
				// Single chunk (no more data)
				sb.WriteString(fmt.Sprintf("\033_Ga=T,f=100,c=%d,q=2,i=1;%s\033\\", cols, chunk))
			} else {
				// More chunks coming (m=1)
				sb.WriteString(fmt.Sprintf("\033_Ga=T,f=100,c=%d,q=2,i=1,m=1;%s\033\\", cols, chunk))
			}
		} else if end >= len(encoded) {
			// Last chunk (m=0)
			sb.WriteString(fmt.Sprintf("\033_Gm=0;%s\033\\", chunk))
		} else {
			// Middle chunk (m=1)
			sb.WriteString(fmt.Sprintf("\033_Gm=1;%s\033\\", chunk))
		}
	}

	return sb.String()
}

type model struct {
	state           menuState
	prevState       menuState
	cursor          int
	menuItems       []string
	projects        []string
	projectPaths    []string
	selectedProject string
	selectedPath    string
	projectAction   projectAction
	git             gitInfo // repo summary for the Git submenu
	activeSessions  map[string]bool
	sessionNames    []string
	selectedSession string
	textInput       textinput.Model
	inputPrompt     string
	message         string
	messageType     string // "success", "error", "info"
	numBuf          string // digits typed so far for multi-digit menu selection
	numSeq          int    // sequence number to ignore stale numTimeoutMsgs
	width           int
	height          int
}

// numTimeoutMsg fires when the user pauses after typing a digit
type numTimeoutMsg struct{ seq int }

// numEntryTimeout is how long to wait for a second digit before selecting
const numEntryTimeout = 700 * time.Millisecond

func initialModel() model {
	h, _ := os.Hostname()
	// macOS often returns "dev.local" (mDNS) instead of "dev.lan" (DNS)
	if h == "dev.local" || h == "dev" {
		h = "dev.lan"
	}
	hostname = h

	// Get the path to the executable to find the logo
	if exe, err := os.Executable(); err == nil {
		execPath = filepath.Dir(exe)
		execFullPath = exe
	}

	ti := textinput.New()
	ti.Placeholder = "project-name"
	ti.CharLimit = 64
	ti.Width = 30

	return model{
		state:     stateMain,
		cursor:    0,
		width:     80,
		height:    24,
		textInput: ti,
	}
}

func (m model) Init() tea.Cmd {
	return tea.ClearScreen
}

// Messages
type cmdFinishedMsg struct {
	output string
	err    error
}

type returnToMenuMsg struct{}

func runCommand(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(name, args...)
		output, err := cmd.CombinedOutput()
		return cmdFinishedMsg{output: string(output), err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle text input states separately
		if m.state == stateSetupProject {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.textInput.Reset()
				return m.goBack(), nil
			case "enter":
				projectName := m.textInput.Value()
				if projectName == "" {
					m.message = "Project name cannot be empty"
					m.messageType = "error"
					return m, nil
				}
				return m.createProject(projectName)
			default:
				var cmd tea.Cmd
				m.textInput, cmd = m.textInput.Update(msg)
				return m, cmd
			}
		}

		// Clear message on any keypress
		m.message = ""
		m.messageType = ""

		key := msg.String()
		if isAllDigits(key) {
			// Fast typing or a paste can deliver several digits in one key
			// message ("13"); feed them through one at a time.
			var cmd tea.Cmd
			cur := m
			for _, r := range key {
				var next tea.Model
				next, cmd = cur.handleDigit(string(r))
				cur = next.(model)
			}
			return cur, cmd
		}
		// Any non-digit key abandons a pending multi-digit entry
		// (enter still selects the item the digits already moved the cursor to)
		m.numBuf = ""
		m.numSeq++

		switch key {
		case "ctrl+c", "q":
			if m.state == stateMain {
				return m, tea.Quit
			}
			// Go back to previous menu
			return m.goBack(), nil

		case "esc":
			return m.goBack(), nil

		case "up", "k":
			// Column lists run top-to-bottom then into the next column, so
			// up/down are linear there too
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			items := m.getMenuItems()
			if m.cursor < len(items)-1 {
				m.cursor++
			}

		case "left", "h":
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				rows := m.columnRows(len(m.getMenuItems()))
				if m.cursor >= rows {
					m.cursor -= rows
				}
			}

		case "right", "l":
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				n := len(m.getMenuItems())
				rows := m.columnRows(n)
				if m.cursor+rows < n {
					m.cursor += rows
				} else if m.cursor/rows < (n-1)/rows {
					// Next column is shorter than this row: land on its last item
					m.cursor = n - 1
				}
			}

		case "enter", " ":
			return m.handleSelection()
		}

	case numTimeoutMsg:
		// Only act if no further digit/key arrived since the timer was started
		if msg.seq == m.numSeq && m.numBuf != "" {
			m.numBuf = ""
			m.numSeq++
			return m.handleSelection()
		}

	case returnToMenuMsg:
		m.state = stateMain
		m.cursor = 0
		m.message = ""
		m.messageType = ""
		m.activeSessions = tmuxListSessions()
		return m, nil

	case cmdFinishedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Error: %v\n%s", msg.err, msg.output)
			m.messageType = "error"
		} else {
			m.message = msg.output
			m.messageType = "success"
		}
		if m.state == stateGitMenu {
			m.git = loadGitInfo(m.selectedPath)
			if m.cursor >= len(m.getMenuItems()) {
				m.cursor = 0
			}
		}
	}

	return m, nil
}

// The trailing "Back" item is pinned to a fixed number so it sits on the same
// key in every menu: 9 in short menus, 99 in long ones (10+ items). The main
// menu has no Back; Exit keeps its natural number so a repeated 9 can't exit.
func pinnedBack(items []string) (index, number int) {
	if len(items) == 0 || len(items) >= 99 {
		return -1, 0
	}
	last := len(items) - 1
	switch items[last] {
	case "Back", "Back to menu":
	default:
		return -1, 0
	}
	if len(items) < 10 {
		return last, 9
	}
	return last, 99
}

// menuNumber is the number shown for (and typed to pick) item i
func menuNumber(items []string, i int) int {
	if idx, n := pinnedBack(items); i == idx {
		return n
	}
	return i + 1
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// handleDigit accumulates typed digits so menus with 10+ items can be
// selected by number. If another digit could still form a valid item number,
// it moves the cursor and waits briefly for it; otherwise it selects at once.
func (m model) handleDigit(key string) (tea.Model, tea.Cmd) {
	items := m.getMenuItems()
	buf := m.numBuf + key
	n := 0
	for _, c := range buf {
		n = n*10 + int(c-'0')
	}
	m.numSeq++

	pinned, pinNum := pinnedBack(items)
	if pinned >= 0 && n == pinNum {
		m.numBuf = ""
		m.cursor = pinned
		return m.handleSelection()
	}
	// With Back pinned, the highest regular number is len(items)-1
	maxN := len(items)
	if pinned >= 0 {
		maxN = len(items) - 1
	}

	if n < 1 || n > maxN {
		// Out of range: discard the entry
		m.numBuf = ""
		return m, nil
	}

	m.cursor = n - 1
	// A further digit could still be valid: wait for it ("9" may become "99")
	if n*10 <= maxN || (pinNum == 99 && n == 9) {
		// A further digit could still be valid: wait for it
		m.numBuf = buf
		seq := m.numSeq
		return m, tea.Tick(numEntryTimeout, func(time.Time) tea.Msg {
			return numTimeoutMsg{seq: seq}
		})
	}

	m.numBuf = ""
	return m.handleSelection()
}

func (m model) goBack() model {
	switch m.state {
	case stateBrowseProjects, stateSetupProject, stateTools, stateSessions, stateHelp:
		m.state = stateMain
	case stateSessionActions:
		m.state = stateSessions
	case stateProjectActions:
		m.state = stateBrowseProjects
	case stateGitMenu:
		m.state = stateProjectActions
	case stateSetupProjectConfirm:
		m.state = stateSetupProject
	case stateQuickAccess, stateDevTools, statePortAuthority, stateSystemMaintenance, stateNpmUtilities:
		m.state = stateTools
	case stateSelectProject:
		switch m.projectAction {
		case actionPrismaStudio:
			m.state = stateQuickAccess
		case actionRemoveNodeModules:
			m.state = stateSystemMaintenance
		default:
			m.state = stateNpmUtilities
		}
	default:
		m.state = stateMain
	}
	m.cursor = 0
	return m
}

func (m model) getMenuItems() []string {
	switch m.state {
	case stateMain:
		items := []string{}
		if hostname != "dev.lan" {
			items = append(items, "Connect to dev")
		}
		items = append(items, "Browse Projects", "Setup New Project", "Tools")
		if hostname != "mac" {
			items = append(items, "Connect to mac")
		}
		if hostname == "dev.lan" && hasTmux() {
			items = append(items, "Sessions")
		}
		return append(items, "Help", "Exit")

	case stateBrowseProjects:
		items := m.projects
		return append(items, "Back to menu")

	case stateProjectActions:
		var items []string
		if !hasTmux() {
			items = []string{"Claude-logged", "Open"}
		} else {
			sessionName := sanitizeTmuxName(m.selectedProject)
			if m.activeSessions[sessionName] {
				items = []string{"Attach", "Claude-logged", "New shell window", "Kill session"}
			} else {
				items = []string{"Claude-logged", "Open"}
			}
		}
		items = append(items, "Open in VS Code", "Git", "Back")
		return items

	case stateGitMenu:
		if !m.git.isRepo {
			return []string{"Git init", "Back"}
		}
		items := []string{"Git status", "Git log"}
		if m.git.remote != "" {
			items = append(items, "Git pull", "Git push")
		}
		return append(items, "Back")

	case stateSessions:
		items := m.sessionNames
		if len(items) == 0 {
			return []string{"Back"}
		}
		return append(items, "Back")

	case stateHelp:
		return []string{"Back"}

	case stateSessionActions:
		return []string{"Resume", "Kill session", "Back"}

	case stateSetupProjectConfirm:
		return []string{"Start working here", "Launch claude-logged", "Back to menu"}

	case stateTools:
		return []string{"Quick Access", "Dev Tools", "Port Authority", "System Maintenance", "NPM Utilities", "Back"}

	case stateQuickAccess:
		if hostname == "mac" {
			return []string{"SSH to MacBookPro", "Open GitHub", "Prisma Studio (select project)", "PostgreSQL shell", "Back"}
		}
		return []string{"SSH to dev", "Open GitHub", "Prisma Studio (select project)", "PostgreSQL shell", "Back"}

	case stateDevTools:
		return []string{"Kill process on port", "Check port usage", "Start ngrok", "Git status (all projects)", "Git pull (all projects)", "Back"}

	case statePortAuthority:
		return []string{"Check project ports", "Setup ports for project", "Update project port", "View all registered ports", "Open dashboard", "Back"}

	case stateSystemMaintenance:
		return []string{"Docker cleanup", "Homebrew update", "Clear npm cache", "Remove node_modules (select project)", "Clear all caches", "Back"}

	case stateNpmUtilities:
		return []string{"npm audit", "npm outdated", "npm update", "npm dedupe", "npm install", "Check outdated (all)", "Back"}

	case stateSelectProject:
		items := m.projects
		return append(items, "Back")
	}

	return []string{}
}

func (m model) handleSelection() (model, tea.Cmd) {
	items := m.getMenuItems()
	if m.cursor >= len(items) {
		return m, nil
	}

	selected := items[m.cursor]

	switch m.state {
	case stateMain:
		return m.handleMainMenu(selected)
	case stateBrowseProjects:
		return m.handleBrowseProjects(selected)
	case stateProjectActions:
		return m.handleProjectActions(selected)
	case stateSessions:
		return m.handleSessions(selected)
	case stateHelp:
		return m.goBack(), nil
	case stateGitMenu:
		return m.handleGitMenu(selected)
	case stateSessionActions:
		return m.handleSessionActions(selected)
	case stateSetupProjectConfirm:
		return m.handleSetupConfirm(selected)
	case stateTools:
		return m.handleToolsMenu(selected)
	case stateQuickAccess:
		return m.handleQuickAccess(selected)
	case stateDevTools:
		return m.handleDevTools(selected)
	case statePortAuthority:
		return m.handlePortAuthority(selected)
	case stateSystemMaintenance:
		return m.handleSystemMaintenance(selected)
	case stateNpmUtilities:
		return m.handleNpmUtilities(selected)
	case stateSelectProject:
		return m.handleSelectProject(selected)
	}

	return m, nil
}

func (m model) handleMainMenu(selected string) (model, tea.Cmd) {
	switch selected {
	case "Connect to dev":
		return m, execAndQuit("ssh", "dev")
	case "Connect to mac":
		return m, execAndQuit("ssh", "dev@mac.lan")
	case "Browse Projects":
		m.state = stateBrowseProjects
		m.cursor = 0
		m.loadProjects(false)
	case "Setup New Project":
		m.state = stateSetupProject
		m.cursor = 0
		m.textInput.Reset()
		m.textInput.Focus()
		return m, textinput.Blink
	case "Tools":
		m.state = stateTools
		m.cursor = 0
	case "Sessions":
		m.state = stateSessions
		m.cursor = 0
		m.loadSessions()
	case "Help":
		m.state = stateHelp
		m.cursor = 0
	case "Exit":
		fmt.Println("\n" + successStyle.Render("Have a great session!"))
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) loadProjects(packageJsonOnly bool) {
	m.projects = []string{}
	m.projectPaths = []string{}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			path := filepath.Join(projectsDir, entry.Name())

			if packageJsonOnly {
				// Check if has package.json
				if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
					m.projects = append(m.projects, entry.Name())
					m.projectPaths = append(m.projectPaths, path)
				}

				// Check subdirectories for monorepos
				subEntries, _ := os.ReadDir(path)
				for _, sub := range subEntries {
					if sub.IsDir() {
						subPath := filepath.Join(path, sub.Name())
						if _, err := os.Stat(filepath.Join(subPath, "package.json")); err == nil {
							m.projects = append(m.projects, entry.Name()+"/"+sub.Name())
							m.projectPaths = append(m.projectPaths, subPath)
						}
					}
				}
			} else {
				m.projects = append(m.projects, entry.Name())
				m.projectPaths = append(m.projectPaths, path)
			}
		}
	}

	if !packageJsonOnly {
		m.activeSessions = tmuxListSessions()
	}
}

func (m model) handleBrowseProjects(selected string) (model, tea.Cmd) {
	if selected == "Back to menu" {
		return m.goBack(), nil
	}

	// Find selected project
	for i, proj := range m.projects {
		if proj == selected {
			m.selectedProject = proj
			m.selectedPath = m.projectPaths[i]
			m.state = stateProjectActions
			m.cursor = 0
			m.activeSessions = tmuxListSessions()
			return m, nil
		}
	}

	return m, nil
}

func (m model) handleProjectActions(selected string) (model, tea.Cmd) {
	sessionName := sanitizeTmuxName(m.selectedProject)
	exists := tmuxSessionExists(sessionName)

	switch selected {
	case "Attach", "Open":
		if !hasTmux() {
			return m, execInDirAndReturn(m.selectedPath, "zsh")
		}
		if exists {
			if isInsideTmux() {
				return m, func() tea.Msg {
					exec.Command("tmux", "switch-client", "-t", sessionName).Run()
					return tea.Quit()
				}
			}
			return m, execAndQuit("tmux", "attach", "-t", sessionName)
		}
		// Create new session
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath).Run()
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath)

	case "New shell window":
		// A session already exists (often running Claude): open a plain shell
		// in a new window of that session, in the project folder, and go there.
		if !hasTmux() || !exists {
			return m.handleProjectActions("Open")
		}
		if err := exec.Command("tmux", "new-window", "-t", sessionName, "-c", m.selectedPath).Run(); err != nil {
			m.message = fmt.Sprintf("Could not open window in '%s': %v", sessionName, err)
			m.messageType = "error"
			return m, nil
		}
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "attach", "-t", sessionName)

	case "Claude-logged":
		if !hasTmux() {
			return m, execInDirAndReturn(m.selectedPath, "zsh", "-lc", claudeSessionCmd())
		}
		if exists {
			// Add new window in existing session
			exec.Command("tmux", "new-window", "-t", sessionName, "-c", m.selectedPath, "zsh", "-lc", claudeLoggedCmd(sessionName)).Run()
			if isInsideTmux() {
				return m, switchAndWaitForSession(sessionName)
			}
			return m, execAndQuit("tmux", "attach", "-t", sessionName)
		}
		// Create new session running claude-logged
		if isInsideTmux() {
			exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath, "zsh", "-lc", claudeLoggedCmd(sessionName)).Run()
			return m, switchAndWaitForSession(sessionName)
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath, "zsh", "-lc", claudeLoggedCmd(sessionName))

	case "Kill session":
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
		m.activeSessions = tmuxListSessions()
		m.message = fmt.Sprintf("Killed tmux session '%s'", sessionName)
		m.messageType = "success"
		return m.goBack(), nil

	case "Open in VS Code":
		m.message = "Opening in VS Code..."
		m.messageType = "info"
		return m, openInVSCode(m.selectedProject, m.selectedPath)

	case "Git":
		m.git = loadGitInfo(m.selectedPath)
		m.state = stateGitMenu
		m.cursor = 0
		return m, nil

	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

// --- Open in VS Code ---
//
// When commandy runs on dev.lan over SSH, VS Code should open on the Mac the
// user is sitting at, via dev.lan's SMB "Projects" share (mounted by macOS at
// /Volumes/Projects). When commandy runs on that Mac itself, open locally.

const vsCodeBin = "/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code"

// sshClientIP returns the address of the machine the user is connecting from,
// or "" when commandy is running locally (console or no SSH).
func sshClientIP() string {
	if conn := os.Getenv("SSH_CONNECTION"); conn != "" {
		return strings.Fields(conn)[0]
	}
	// Inside tmux the environment may predate this connection: look up the
	// attached client's tty in `who` instead.
	if isInsideTmux() {
		out, err := exec.Command("tmux", "display-message", "-p", "#{client_tty}").Output()
		if err == nil {
			tty := strings.TrimPrefix(strings.TrimSpace(string(out)), "/dev/")
			who, _ := exec.Command("who").Output()
			for _, line := range strings.Split(string(who), "\n") {
				f := strings.Fields(line)
				if len(f) >= 2 && f[1] == tty {
					if i := strings.LastIndex(line, "("); i >= 0 && strings.HasSuffix(line, ")") {
						return line[i+1 : len(line)-1]
					}
				}
			}
		}
	}
	return ""
}

// clientMacHost maps a connecting IP to an SSH host alias from ~/.ssh/config
func clientMacHost(ip string) string {
	switch ip {
	case "172.16.1.132":
		return "devbook"
	case "172.16.1.113":
		return "mac"
	case "172.16.1.190":
		return "mac.lan"
	}
	return ip
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func openInVSCode(project, path string) tea.Cmd {
	return func() tea.Msg {
		ip := sshClientIP()
		if ip == "" {
			// Local: open the folder directly
			var cmd *exec.Cmd
			if _, err := os.Stat(vsCodeBin); err == nil {
				cmd = exec.Command(vsCodeBin, path)
			} else if codePath, err := exec.LookPath("code"); err == nil {
				cmd = exec.Command(codePath, path)
			} else {
				cmd = exec.Command("open", "-a", "Visual Studio Code", path)
			}
			if out, err := cmd.CombinedOutput(); err != nil {
				return cmdFinishedMsg{err: err, output: strings.TrimSpace(string(out))}
			}
			return cmdFinishedMsg{output: fmt.Sprintf("Opened %s in VS Code", project)}
		}

		host := clientMacHost(ip)
		share := fmt.Sprintf("smb://%s/Projects", hostname)
		mounted := "/Volumes/Projects/" + project
		script := fmt.Sprintf(`p=%s
if [ ! -d "$p" ]; then
  open %s
  for i in $(seq 1 20); do [ -d "$p" ] && break; sleep 0.5; done
fi
[ -d "$p" ] || { echo "Share not mounted on this Mac: $p (open %s in Finder first)"; exit 1; }
if [ -x %s ]; then %s "$p"; else open -a "Visual Studio Code" "$p"; fi`,
			shellQuote(mounted), shellQuote(share), share, shellQuote(vsCodeBin), shellQuote(vsCodeBin))

		cmd := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
			"-o", "ControlMaster=auto", "-o", "ControlPath="+filepath.Join(os.Getenv("HOME"), ".ssh/cm/%r@%h:%p"),
			"-o", "ControlPersist=10m", host, script)
		if out, err := cmd.CombinedOutput(); err != nil {
			return cmdFinishedMsg{err: fmt.Errorf("could not open on %s", host), output: strings.TrimSpace(string(out))}
		}
		return cmdFinishedMsg{output: fmt.Sprintf("Opened %s in VS Code on %s (%s)", project, host, mounted)}
	}
}

// gitInfo summarises a project's repository for the Git submenu
type gitInfo struct {
	isRepo      bool
	branch      string
	lastRel     string // e.g. "3 days ago"
	lastAbs     string // e.g. "2026-10-16 14:02"
	lastSubject string
	dirty       int    // changed/untracked files
	remote      string // first remote's URL, "" if none
	hasUpstream bool
	ahead       int
	behind      int
}

func gitOut(path string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", path}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func loadGitInfo(path string) gitInfo {
	var g gitInfo
	if !isGitRepo(path) {
		return g
	}
	g.isRepo = true

	if b, err := gitOut(path, "symbolic-ref", "--short", "HEAD"); err == nil {
		g.branch = b
	} else if b, err := gitOut(path, "rev-parse", "--short", "HEAD"); err == nil {
		g.branch = "detached @ " + b
	}

	if out, err := gitOut(path, "log", "-1", "--format=%cr%n%cd%n%s", "--date=format:%Y-%m-%d %H:%M"); err == nil {
		parts := strings.SplitN(out, "\n", 3)
		if len(parts) == 3 {
			g.lastRel, g.lastAbs, g.lastSubject = parts[0], parts[1], parts[2]
		}
	}

	if out, err := gitOut(path, "status", "--porcelain"); err == nil && out != "" {
		g.dirty = len(strings.Split(out, "\n"))
	}

	if remotes, err := gitOut(path, "remote"); err == nil && remotes != "" {
		name := strings.Split(remotes, "\n")[0]
		if url, err := gitOut(path, "remote", "get-url", name); err == nil {
			g.remote = url
		} else {
			g.remote = name
		}
	}

	if out, err := gitOut(path, "rev-list", "--left-right", "--count", "@{u}...HEAD"); err == nil {
		g.hasUpstream = true
		fmt.Sscanf(out, "%d %d", &g.behind, &g.ahead)
	}
	return g
}

// renderGitInfo is the summary block shown above the Git submenu
func (m model) renderGitInfo() string {
	g := m.git
	label := func(s string) string { return dimStyle.Render(fmt.Sprintf("  %-12s", s)) }
	var s strings.Builder

	if !g.isRepo {
		s.WriteString(subtitleStyle.Render("  Not a git repository") + "\n")
		s.WriteString(dimStyle.Render("  Git init will create one in ") + m.selectedPath + "\n")
		return s.String()
	}

	if g.lastAbs == "" {
		s.WriteString(label("Last commit") + subtitleStyle.Render("none yet") + "\n")
	} else {
		s.WriteString(label("Last commit") + selectedStyle.Render(g.lastRel) + dimStyle.Render("  ("+g.lastAbs+")") + "\n")
		s.WriteString(label("") + normalStyle.Render(g.lastSubject) + "\n")
	}

	branch := g.branch
	if g.hasUpstream {
		switch {
		case g.ahead > 0 && g.behind > 0:
			branch += fmt.Sprintf("  ↑%d ↓%d", g.ahead, g.behind)
		case g.ahead > 0:
			branch += fmt.Sprintf("  ↑%d to push", g.ahead)
		case g.behind > 0:
			branch += fmt.Sprintf("  ↓%d to pull", g.behind)
		default:
			branch += "  in sync"
		}
	} else if g.remote != "" {
		branch += "  (no upstream)"
	}
	s.WriteString(label("Branch") + normalStyle.Render(branch) + "\n")

	if g.dirty > 0 {
		s.WriteString(label("Changes") + errorStyle.Render(fmt.Sprintf("%d uncommitted file(s)", g.dirty)) + "\n")
	} else {
		s.WriteString(label("Changes") + successStyle.Render("clean") + "\n")
	}

	if g.remote != "" {
		s.WriteString(label("Remote") + dimStyle.Render(g.remote) + "\n")
	} else {
		s.WriteString(label("Remote") + subtitleStyle.Render("none (add one before push/pull)") + "\n")
	}
	return s.String()
}

func (m model) handleGitMenu(selected string) (model, tea.Cmd) {
	path := m.selectedPath
	switch selected {
	case "Git init":
		out, err := exec.Command("git", "-C", path, "init").CombinedOutput()
		if err != nil {
			m.message = fmt.Sprintf("git init failed: %v\n%s", err, strings.TrimSpace(string(out)))
			m.messageType = "error"
		} else {
			m.message = strings.TrimSpace(string(out))
			m.messageType = "success"
		}
		m.git = loadGitInfo(path)
		m.cursor = 0
		return m, nil

	case "Git status":
		m.message = "Checking..."
		m.messageType = "info"
		return m, gitCommandInProject(path, "status", "--short", "--branch")

	case "Git log":
		m.message = "Loading..."
		m.messageType = "info"
		return m, gitCommandInProject(path, "log", "-10", "--date=relative", "--format=%h  %ad  %s")

	case "Git pull":
		m.message = "Pulling..."
		m.messageType = "info"
		return m, gitPullProject(path)

	case "Git push":
		m.message = "Pushing..."
		m.messageType = "info"
		if m.git.hasUpstream {
			return m, gitCommandInProject(path, "push")
		}
		return m, gitCommandInProject(path, "push", "-u", "origin", m.git.branch)

	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

// gitCommandInProject runs a git command in a project and reports its output
func gitCommandInProject(path string, args ...string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", append([]string{"-C", path}, args...)...).CombinedOutput()
		output := strings.TrimSpace(string(out))
		if output == "" {
			output = "(nothing to report)"
		}
		if err != nil {
			return cmdFinishedMsg{err: fmt.Errorf("git %s failed", args[0]), output: output}
		}
		return cmdFinishedMsg{output: fmt.Sprintf("git %s:\n%s", args[0], output)}
	}
}

// isGitRepo reports whether path has a .git directory (or file, for worktrees)
func isGitRepo(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// gitPullProject runs `git pull` in a single project and reports the result
func gitPullProject(path string) tea.Cmd {
	return func() tea.Msg {
		branch := "?"
		if out, err := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
			branch = strings.TrimSpace(string(out))
		}

		cmd := exec.Command("git", "-C", path, "pull")
		out, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(out))
		if err != nil {
			return cmdFinishedMsg{err: fmt.Errorf("git pull failed on %s", branch), output: output}
		}
		return cmdFinishedMsg{output: fmt.Sprintf("git pull (%s):\n%s", branch, output)}
	}
}

func (m *model) loadSessions() {
	m.sessionNames = []string{}
	sessions := tmuxListSessions()
	for name := range sessions {
		m.sessionNames = append(m.sessionNames, name)
	}
	// Map order is random; keep the list stable so numbered entry is predictable
	sort.Strings(m.sessionNames)
}

// currentTmuxSession returns the session commandy is running inside, or ""
func currentTmuxSession() string {
	if !isInsideTmux() {
		return ""
	}
	out, err := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (m model) handleSessions(selected string) (model, tea.Cmd) {
	if selected == "Back" {
		return m.goBack(), nil
	}

	m.selectedSession = selected
	m.state = stateSessionActions
	m.cursor = 0
	return m, nil
}

func (m model) handleSessionActions(selected string) (model, tea.Cmd) {
	switch selected {
	case "Resume":
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "switch-client", "-t", m.selectedSession).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "attach", "-t", m.selectedSession)
	case "Kill session":
		// Killing the session commandy itself runs in would take commandy down
		// with it (the pane dies), which looks like commandy "just closing".
		if cur := currentTmuxSession(); cur != "" && cur == m.selectedSession {
			m.message = fmt.Sprintf("'%s' is the session commandy is running in. Detach first (Ctrl+A d) and kill it from outside, or exit its windows.", cur)
			m.messageType = "error"
			return m, nil
		}
		if out, err := exec.Command("tmux", "kill-session", "-t", m.selectedSession).CombinedOutput(); err != nil {
			m.message = fmt.Sprintf("Could not kill '%s': %s", m.selectedSession, strings.TrimSpace(string(out)))
			m.messageType = "error"
			return m, nil
		}
		m.message = fmt.Sprintf("Killed tmux session '%s'", m.selectedSession)
		m.messageType = "success"
		m.loadSessions()
		if len(m.sessionNames) == 0 {
			m.state = stateSessions
			m.cursor = 0
			return m, nil
		}
		return m.goBack(), nil
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) createProject(name string) (model, tea.Cmd) {
	projectPath := filepath.Join(projectsDir, name)

	// Check if already exists
	if _, err := os.Stat(projectPath); err == nil {
		m.message = "Project already exists!"
		m.messageType = "error"
		return m, nil
	}

	// Create directory
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		m.message = fmt.Sprintf("Error creating directory: %v", err)
		m.messageType = "error"
		return m, nil
	}

	// Initialize git
	cmd := exec.Command("git", "init")
	cmd.Dir = projectPath
	if err := cmd.Run(); err != nil {
		m.message = fmt.Sprintf("Error initializing git: %v", err)
		m.messageType = "error"
		return m, nil
	}

	// Success - go to confirmation
	m.selectedProject = name
	m.selectedPath = projectPath
	m.state = stateSetupProjectConfirm
	m.cursor = 0
	m.textInput.Reset()
	m.textInput.Blur()
	m.message = fmt.Sprintf("Project '%s' created at %s", name, projectPath)
	m.messageType = "success"

	return m, nil
}

func (m model) handleSetupConfirm(selected string) (model, tea.Cmd) {
	sessionName := sanitizeTmuxName(m.selectedProject)

	switch selected {
	case "Start working here":
		if !hasTmux() {
			return m, execInDirAndReturn(m.selectedPath, "zsh")
		}
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath).Run()
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath)
	case "Launch claude-logged":
		if !hasTmux() {
			return m, execInDirAndReturn(m.selectedPath, "zsh", "-lc", claudeSessionCmd())
		}
		if isInsideTmux() {
			exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath, "zsh", "-lc", claudeLoggedCmd(sessionName)).Run()
			return m, switchAndWaitForSession(sessionName)
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath, "zsh", "-lc", claudeLoggedCmd(sessionName))
	case "Back to menu":
		m.state = stateMain
		m.cursor = 0
		return m, nil
	}
	return m, nil
}

func (m model) handleToolsMenu(selected string) (model, tea.Cmd) {
	switch selected {
	case "Quick Access":
		m.state = stateQuickAccess
		m.cursor = 0
	case "Dev Tools":
		m.state = stateDevTools
		m.cursor = 0
	case "Port Authority":
		m.state = statePortAuthority
		m.cursor = 0
	case "System Maintenance":
		m.state = stateSystemMaintenance
		m.cursor = 0
	case "NPM Utilities":
		m.state = stateNpmUtilities
		m.cursor = 0
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) handleQuickAccess(selected string) (model, tea.Cmd) {
	switch selected {
	case "SSH to MacBookPro":
		return m, execAndQuit("ssh", "alexander@MacBookPro.local")
	case "SSH to dev":
		return m, execAndQuit("ssh", "dev")
	case "Open GitHub":
		exec.Command("open", "https://github.com").Start()
		m.message = "Opened GitHub in browser"
		m.messageType = "success"
		return m, nil
	case "Prisma Studio (select project)":
		m.projectAction = actionPrismaStudio
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
		return m, nil
	case "PostgreSQL shell":
		return m, execAndQuit("psql", "postgresql://postgres:postgres@localhost:5432/fintrac")
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) handleDevTools(selected string) (model, tea.Cmd) {
	switch selected {
	case "Kill process on port":
		// For simplicity, we'll show common ports status
		m.message = "Use: lsof -ti:PORT | xargs kill -9"
		m.messageType = "info"
		return m, nil
	case "Check port usage":
		return m, checkPorts()
	case "Start ngrok":
		return m, execAndQuit("ngrok", "http", "3012")
	case "Git status (all projects)":
		return m, gitStatusAll()
	case "Git pull (all projects)":
		return m, gitPullAll()
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) handlePortAuthority(selected string) (model, tea.Cmd) {
	switch selected {
	case "Open dashboard":
		exec.Command("open", portAuthorityDashboard).Start()
		m.message = "Opened Port Authority dashboard"
		m.messageType = "success"
		return m, nil
	case "View all registered ports":
		return m, fetchPorts()
	case "Back":
		return m.goBack(), nil
	default:
		m.message = "Feature available via Port Authority dashboard"
		m.messageType = "info"
		return m, nil
	}
}

func (m model) handleSystemMaintenance(selected string) (model, tea.Cmd) {
	switch selected {
	case "Docker cleanup":
		return m, dockerCleanup()
	case "Homebrew update":
		return m, brewUpdate()
	case "Clear npm cache":
		return m, execAndReturn("npm", "cache", "clean", "--force")
	case "Remove node_modules (select project)":
		m.projectAction = actionRemoveNodeModules
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
		return m, nil
	case "Clear all caches":
		return m, clearAllCaches()
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) handleNpmUtilities(selected string) (model, tea.Cmd) {
	switch selected {
	case "npm audit":
		m.projectAction = actionNpmAudit
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
	case "npm outdated":
		m.projectAction = actionNpmOutdated
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
	case "npm update":
		m.projectAction = actionNpmUpdate
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
	case "npm dedupe":
		m.projectAction = actionNpmDedupe
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
	case "npm install":
		m.projectAction = actionNpmInstall
		m.state = stateSelectProject
		m.cursor = 0
		m.loadProjects(true)
	case "Check outdated (all)":
		return m, npmOutdatedAll()
	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m model) handleSelectProject(selected string) (model, tea.Cmd) {
	if selected == "Back" {
		return m.goBack(), nil
	}

	// Find selected project path
	var projectPath string
	for i, proj := range m.projects {
		if proj == selected {
			projectPath = m.projectPaths[i]
			break
		}
	}

	if projectPath == "" {
		return m, nil
	}

	switch m.projectAction {
	case actionPrismaStudio:
		return m, execInDirAndQuit(projectPath, "npx", "prisma", "studio")
	case actionRemoveNodeModules:
		nodeModules := filepath.Join(projectPath, "node_modules")
		os.RemoveAll(nodeModules)
		m.message = fmt.Sprintf("Removed node_modules from %s", selected)
		m.messageType = "success"
		return m.goBack(), nil
	case actionNpmAudit:
		return m, execInDir(projectPath, "npm", "audit")
	case actionNpmOutdated:
		return m, execInDir(projectPath, "npm", "outdated")
	case actionNpmUpdate:
		return m, execInDir(projectPath, "npm", "update")
	case actionNpmDedupe:
		return m, execInDir(projectPath, "npm", "dedupe")
	case actionNpmInstall:
		return m, execInDir(projectPath, "npm", "install")
	}

	return m, nil
}

// Command helpers
func execAndQuit(name string, args ...string) tea.Cmd {
	return tea.ExecProcess(exec.Command(name, args...), func(err error) tea.Msg {
		return tea.Quit()
	})
}

func execAndReturn(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(name, args...)
		output, err := cmd.CombinedOutput()
		return cmdFinishedMsg{output: string(output), err: err}
	}
}

func execInDir(dir, name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		return cmdFinishedMsg{output: string(output), err: err}
	}
}

func execInDirAndQuit(dir, name string, args ...string) tea.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return tea.Quit()
	})
}

func claudeLoggerAPI() string {
	api := os.Getenv("CLAUDE_LOGGER_API")
	if api == "" {
		api = "http://zynx.lan:3000"
	}
	return api
}

// claudeSessionCmd runs claude, then uploads the JSONL session log
// that Claude Code writes to ~/.claude/projects/ to the logger API.
func claudeSessionCmd() string {
	return fmt.Sprintf(`claude
CLAUDE_DIR="$HOME/.claude/projects/$(echo "$PWD" | tr '/' '-')"
LATEST=$(ls -t "$CLAUDE_DIR"/*.jsonl 2>/dev/null | head -1)
if [ -n "$LATEST" ]; then
  SID=$(basename "$LATEST" .jsonl)
  curl -sf -X POST "%s/api/sessions/$SID" -H "X-Project: $PWD" --data-binary "@$LATEST" > /dev/null 2>&1 || true
fi`, claudeLoggerAPI())
}

// claudeLoggedCmd returns the session command with a tmux wait-for signal at the end.
// The signal allows the calling commandy instance to know when the session finishes.
func claudeLoggedCmd(sessionName string) string {
	return claudeSessionCmd() + fmt.Sprintf("\ntmux wait-for -S 'done-%s'", sessionName)
}

func execInDirAndReturn(dir, name string, args ...string) tea.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return returnToMenuMsg{}
	})
}

// switchAndWaitForSession switches to a tmux session and blocks until it signals
// completion. Both commands run inside tea.ExecProcess so the TUI is properly
// suspended before the switch and resumed after the session ends.
func switchAndWaitForSession(sessionName string) tea.Cmd {
	cmd := exec.Command("sh", "-c", fmt.Sprintf(
		"tmux switch-client -t '%s' && tmux wait-for 'done-%s'",
		sessionName, sessionName))
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return returnToMenuMsg{}
	})
}

// Tmux helpers

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func sanitizeTmuxName(name string) string {
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, ":", "-")
	return name
}

func isInsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

func tmuxListSessions() map[string]bool {
	sessions := make(map[string]bool)
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		return sessions
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			sessions[line] = true
		}
	}
	return sessions
}

func tmuxSessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

func checkPorts() tea.Cmd {
	return func() tea.Msg {
		ports := []int{3000, 3012, 5173, 5432, 6379, 8080}
		var results []string

		for _, port := range ports {
			cmd := exec.Command("lsof", "-ti", fmt.Sprintf(":%d", port))
			output, _ := cmd.Output()
			pid := strings.TrimSpace(string(output))
			if pid != "" {
				results = append(results, fmt.Sprintf("Port %d: PID %s", port, pid))
			} else {
				results = append(results, fmt.Sprintf("Port %d: available", port))
			}
		}

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func gitStatusAll() tea.Cmd {
	return func() tea.Msg {
		var results []string
		entries, _ := os.ReadDir(projectsDir)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(projectsDir, entry.Name())
			gitDir := filepath.Join(path, ".git")
			if _, err := os.Stat(gitDir); err != nil {
				continue
			}

			// Get branch
			branchCmd := exec.Command("git", "branch", "--show-current")
			branchCmd.Dir = path
			branchOut, _ := branchCmd.Output()
			branch := strings.TrimSpace(string(branchOut))

			// Get status
			statusCmd := exec.Command("git", "status", "--porcelain")
			statusCmd.Dir = path
			statusOut, _ := statusCmd.Output()

			status := "clean"
			if len(statusOut) > 0 {
				status = "has changes"
			}

			results = append(results, fmt.Sprintf("%s (%s) - %s", entry.Name(), branch, status))
		}

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func gitPullAll() tea.Cmd {
	return func() tea.Msg {
		var results []string
		entries, _ := os.ReadDir(projectsDir)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(projectsDir, entry.Name())
			gitDir := filepath.Join(path, ".git")
			if _, err := os.Stat(gitDir); err != nil {
				continue
			}

			cmd := exec.Command("git", "pull", "--quiet")
			cmd.Dir = path
			err := cmd.Run()

			status := "updated"
			if err != nil {
				status = "failed"
			}

			results = append(results, fmt.Sprintf("%s: %s", entry.Name(), status))
		}

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func fetchPorts() tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("curl", "-s", portAuthorityAPI+"/ports")
		output, err := cmd.Output()
		if err != nil {
			return cmdFinishedMsg{err: err}
		}
		return cmdFinishedMsg{output: string(output)}
	}
}

func dockerCleanup() tea.Cmd {
	return func() tea.Msg {
		var results []string

		cmds := []struct {
			name string
			args []string
		}{
			{"Removing stopped containers", []string{"docker", "container", "prune", "-f"}},
			{"Removing unused images", []string{"docker", "image", "prune", "-f"}},
			{"Removing unused volumes", []string{"docker", "volume", "prune", "-f"}},
			{"Removing unused networks", []string{"docker", "network", "prune", "-f"}},
		}

		for _, c := range cmds {
			results = append(results, c.name+"...")
			cmd := exec.Command(c.args[0], c.args[1:]...)
			cmd.Run()
		}

		// Get final disk usage
		dfCmd := exec.Command("docker", "system", "df")
		dfOut, _ := dfCmd.Output()
		results = append(results, "\n"+string(dfOut))

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func brewUpdate() tea.Cmd {
	return func() tea.Msg {
		var results []string

		steps := []struct {
			name string
			args []string
		}{
			{"Updating Homebrew", []string{"brew", "update"}},
			{"Upgrading packages", []string{"brew", "upgrade"}},
			{"Cleaning up", []string{"brew", "cleanup"}},
		}

		for _, s := range steps {
			results = append(results, s.name+"...")
			cmd := exec.Command(s.args[0], s.args[1:]...)
			output, _ := cmd.CombinedOutput()
			results = append(results, string(output))
		}

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func clearAllCaches() tea.Cmd {
	return func() tea.Msg {
		var results []string

		// npm cache
		results = append(results, "Clearing npm cache...")
		exec.Command("npm", "cache", "clean", "--force").Run()

		// brew cache
		results = append(results, "Clearing Homebrew cache...")
		exec.Command("brew", "cleanup", "-s").Run()

		// .DS_Store files
		results = append(results, "Removing .DS_Store files...")
		exec.Command("find", projectsDir, "-name", ".DS_Store", "-delete").Run()

		results = append(results, "\nAll caches cleared!")

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

func npmOutdatedAll() tea.Cmd {
	return func() tea.Msg {
		var results []string
		entries, _ := os.ReadDir(projectsDir)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(projectsDir, entry.Name())

			// Check main project
			if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
				results = append(results, fmt.Sprintf("\n━━━ %s ━━━", entry.Name()))
				cmd := exec.Command("npm", "outdated")
				cmd.Dir = path
				output, _ := cmd.CombinedOutput()
				if len(output) > 0 {
					results = append(results, string(output))
				} else {
					results = append(results, "No outdated packages")
				}
			}

			// Check subdirectories
			subEntries, _ := os.ReadDir(path)
			for _, sub := range subEntries {
				if !sub.IsDir() {
					continue
				}
				subPath := filepath.Join(path, sub.Name())
				if _, err := os.Stat(filepath.Join(subPath, "package.json")); err == nil {
					results = append(results, fmt.Sprintf("\n━━━ %s/%s ━━━", entry.Name(), sub.Name()))
					cmd := exec.Command("npm", "outdated")
					cmd.Dir = subPath
					output, _ := cmd.CombinedOutput()
					if len(output) > 0 {
						results = append(results, string(output))
					} else {
						results = append(results, "No outdated packages")
					}
				}
			}
		}

		return cmdFinishedMsg{output: strings.Join(results, "\n")}
	}
}

// View
func (m model) View() string {
	var s strings.Builder

	// Banner
	s.WriteString(m.renderBanner())
	s.WriteString("\n")

	// Menu title
	title := m.getMenuTitle()
	if title != "" {
		s.WriteString(headerStyle.Render(title))
		s.WriteString("\n\n")
	}

	// Special handling for text input state
	if m.state == stateSetupProject {
		s.WriteString(headerStyle.Render("Enter new project name:"))
		s.WriteString("\n\n")
		s.WriteString("  " + m.textInput.View())
		s.WriteString("\n")

		// Message
		if m.message != "" {
			s.WriteString("\n")
			switch m.messageType {
			case "success":
				s.WriteString(successStyle.Render(m.message))
			case "error":
				s.WriteString(errorStyle.Render(m.message))
			default:
				s.WriteString(dimStyle.Render(m.message))
			}
			s.WriteString("\n")
		}

		s.WriteString("\n")
		s.WriteString(dimStyle.Render("enter confirm • esc cancel"))
		return s.String()
	}

	// Help guide
	if m.state == stateHelp {
		s.WriteString(renderCopyPasteHelp())
		s.WriteString("\n")
	}

	// Git summary
	if m.state == stateGitMenu {
		s.WriteString(m.renderGitInfo())
		s.WriteString("\n")
	}

	// Empty sessions message
	if m.state == stateSessions && len(m.sessionNames) == 0 {
		s.WriteString(dimStyle.Render("  No active tmux sessions"))
		s.WriteString("\n\n")
	}

	// Menu items
	items := m.getMenuItems()

	// Multi-column layout for project lists
	if m.state == stateBrowseProjects || m.state == stateSelectProject {
		s.WriteString(m.renderColumnMenu(items))
	} else {
		for i, item := range items {
			cursor := "  "
			style := normalStyle
			if i == m.cursor {
				cursor = cursorStyle.Render("> ")
				style = selectedStyle
			}

			num := dimStyle.Render(fmt.Sprintf("%d) ", menuNumber(items, i)))
			s.WriteString(cursor + num + style.Render(item) + "\n")
		}
	}

	// Message
	if m.message != "" {
		s.WriteString("\n")
		switch m.messageType {
		case "success":
			s.WriteString(successStyle.Render(m.message))
		case "error":
			s.WriteString(errorStyle.Render(m.message))
		default:
			s.WriteString(dimStyle.Render(m.message))
		}
		s.WriteString("\n")
	}

	// Help
	s.WriteString("\n")
	if m.numBuf != "" {
		s.WriteString(dimStyle.Render("number: ") + selectedStyle.Render(m.numBuf+"_") + "\n")
	}
	if m.state == stateBrowseProjects || m.state == stateSelectProject {
		s.WriteString(dimStyle.Render("←/→ columns • ↑/↓ navigate • enter select • q/esc back"))
	} else {
		s.WriteString(dimStyle.Render("↑/↓ navigate • enter select • q/esc back"))
	}

	return s.String()
}

func (m model) getMenuTitle() string {
	switch m.state {
	case stateMain:
		return "What would you like to do?"
	case stateBrowseProjects:
		return "Select a project"
	case stateProjectActions:
		return fmt.Sprintf("Project: %s", m.selectedProject)
	case stateSessions:
		if cur := currentTmuxSession(); cur != "" {
			return fmt.Sprintf("Tmux Sessions  (you are in '%s')", cur)
		}
		return "Tmux Sessions"
	case stateHelp:
		return "Help: copy & paste"
	case stateGitMenu:
		return fmt.Sprintf("Git: %s", m.selectedProject)
	case stateSessionActions:
		return fmt.Sprintf("Session: %s", m.selectedSession)
	case stateSetupProject:
		return "Setup New Project"
	case stateSetupProjectConfirm:
		return fmt.Sprintf("Project '%s' created!", m.selectedProject)
	case stateTools:
		return "Tools"
	case stateQuickAccess:
		return "Quick Access"
	case stateDevTools:
		return "Dev Tools"
	case statePortAuthority:
		return "Port Authority"
	case stateSystemMaintenance:
		return "System Maintenance"
	case stateNpmUtilities:
		return "NPM Utilities"
	case stateSelectProject:
		return "Select a project"
	}
	return ""
}

// renderCopyPasteHelp returns the keystroke guide shown in the Help menu.
// Every copy inside tmux goes to the tmux buffer, this machine's clipboard,
// and the clipboard of the Mac you're connected from (via ~/.local/bin/tmux-clip).
func renderCopyPasteHelp() string {
	keyStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true).Width(26)
	var s strings.Builder

	section := func(title string) {
		s.WriteString(subtitleStyle.Render(title) + "\n")
	}
	row := func(key, desc string) {
		s.WriteString("  " + keyStyle.Render(key) + normalStyle.Render(desc) + "\n")
	}

	section("Copy (inside tmux)")
	row("Mouse drag, release", "copy the selection")
	row("Double / triple click", "copy a word / a line")
	row("Ctrl+A  [", "enter copy mode (vi keys)")
	row("   v  then  y", "start selection, then copy")
	row("   Enter", "copy selection (alternative to y)")
	row("   /  n  N", "search forward, next, previous")
	row("   g  G  PgUp  PgDn", "top, bottom, page up / down")
	row("   q  or  Esc", "leave copy mode")
	s.WriteString(dimStyle.Render("  Copies land in the tmux buffer, this Mac's clipboard, and your Mac's clipboard.") + "\n\n")

	section("Paste")
	row("Cmd+V", "paste your Mac's clipboard into the current session")
	row("Ctrl+A  ]", "paste the last tmux copy (shared by all sessions)")
	row("Ctrl+A  =", "pick from earlier tmux copies")
	s.WriteString("\n")

	section("Terminal.app native (bypasses tmux)")
	row("Fn+drag, Cmd+C", "select visible text with Terminal itself (Option/Shift do not bypass tmux)")
	row("View > Allow Mouse Reporting", "untick to make plain drag + Cmd+C native; tick again for tmux")
	s.WriteString("\n")

	section("Sessions")
	row("Ctrl+A  d", "detach (session keeps running)")
	row("Ctrl+A  s", "switch between sessions")
	row("Ctrl+A  |  /  -", "split pane right / below")
	row("Ctrl+A  h j k l", "move between panes")

	return s.String()
}

const (
	menuMinColWidth = 28 // cursor (2) + number (4) + name + session dot
	menuMaxCols     = 3
)

// menuColumns is how many columns the project lists use: three when the
// terminal is wide enough, fewer on narrow ones.
func (m model) menuColumns() int {
	cols := menuMaxCols
	if m.width > 0 {
		for cols > 1 && cols*menuMinColWidth > m.width {
			cols--
		}
	}
	return cols
}

// menuColWidth stretches the columns to fill the terminal width.
func (m model) menuColWidth() int {
	if w := m.width / m.menuColumns(); w > menuMinColWidth {
		return w
	}
	return menuMinColWidth
}

// columnRows is the number of rows needed to lay n items out in columns.
func (m model) columnRows(n int) int {
	cols := m.menuColumns()
	rows := (n + cols - 1) / cols
	if rows < 1 {
		rows = 1
	}
	return rows
}

// renderColumnMenu lays items out column-major (down the first column, then
// the next), matching the cursor movement in Update.
func (m model) renderColumnMenu(items []string) string {
	var s strings.Builder
	cols := m.menuColumns()
	rows := m.columnRows(len(items))
	colWidth := m.menuColWidth()

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			idx := col*rows + row
			if idx >= len(items) {
				break
			}

			cursor := "  "
			style := normalStyle
			if idx == m.cursor {
				cursor = cursorStyle.Render("> ")
				style = selectedStyle
			}
			num := dimStyle.Render(fmt.Sprintf("%2d) ", menuNumber(items, idx)))

			indicator := ""
			indicatorLen := 0
			if m.state == stateBrowseProjects && items[idx] != "Back to menu" {
				if m.activeSessions[sanitizeTmuxName(items[idx])] {
					indicator = " " + lipgloss.NewStyle().Foreground(green).Render("●")
					indicatorLen = 2
				}
			}

			// Keep at least one space before the next column
			item := items[idx]
			maxLen := colWidth - 7 - indicatorLen
			if len(item) > maxLen {
				item = item[:maxLen-3] + "..."
			}
			s.WriteString(cursor + num + style.Render(item) + indicator)

			if col < cols-1 && idx+rows < len(items) {
				s.WriteString(strings.Repeat(" ", colWidth-6-len(item)-indicatorLen))
			}
		}
		s.WriteString("\n")
	}

	return s.String()
}

func buildBanner() string {
	logoPath := filepath.Join(execPath, "commandy2.png")

	// Check if logo exists
	if _, err := os.Stat(logoPath); err == nil {
		// Use chafa to render image as colored text (works with bubbletea redraws)
		cmd := exec.Command("chafa",
			"--format=symbols",
			"--symbols=block+border+diagonal+dot+quad+half+hhalf+vhalf+braille",
			"--color-space=din99d",
			"--dither=ordered",
			"--size=45x22",
			logoPath)
		if output, err := cmd.Output(); err == nil {
			var lines []string
			lines = append(lines, "")
			lines = append(lines, string(output))
			lines = append(lines, subtitleStyle.Render("     ~ Terminal Session Launcher ~"))
			lines = append(lines, "")
			lines = append(lines, "          Host: "+successStyle.Render(hostname))
			lines = append(lines, "")
			return strings.Join(lines, "\n")
		}
	}

	// Fallback to ASCII art if image not available
	border := bannerBorderStyle.Render
	text := bannerTextStyle.Render

	lines := []string{
		"",
		border("╔═══════════════════════════════════════════════════════╗"),
		border("║") + "                                                       " + border("║"),
		border("║") + " " + text("  ####   ####  #    # #    #  ###  #   # ####  #   #") + "   " + border("║"),
		border("║") + " " + text(" #      #    # ##  ## ##  ## #   # ##  # #   #  # # ") + "   " + border("║"),
		border("║") + " " + text(" #      #    # # ## # # ## # ##### # # # #   #   #  ") + "   " + border("║"),
		border("║") + " " + text(" #      #    # #    # #    # #   # #  ## #   #   #  ") + "   " + border("║"),
		border("║") + " " + text("  ####   ####  #    # #    # #   # #   # ####    #  ") + "   " + border("║"),
		border("║") + "                                                       " + border("║"),
		border("║") + "           " + subtitleStyle.Render("~ Terminal Session Launcher ~") + "             " + border("║"),
		border("║") + "                                                       " + border("║"),
		border("║") + "                 Host: " + successStyle.Render(hostname) + strings.Repeat(" ", 23-len(hostname)) + border("║"),
		border("║") + "                                                       " + border("║"),
		border("╚═══════════════════════════════════════════════════════╝"),
		"",
	}

	return strings.Join(lines, "\n")
}

func (m model) renderBanner() string {
	if cachedBanner == "" {
		cachedBanner = buildBanner()
	}
	return cachedBanner
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
