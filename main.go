package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	activeSessions  map[string]bool
	sessionNames    []string
	selectedSession string
	textInput       textinput.Model
	inputPrompt     string
	message         string
	messageType     string // "success", "error", "info"
	width  int
	height int
}

func initialModel() model {
	h, _ := os.Hostname()
	hostname = h

	// Get the path to the executable to find the logo
	if exe, err := os.Executable(); err == nil {
		execPath = filepath.Dir(exe)
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

		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == stateMain {
				return m, tea.Quit
			}
			// Go back to previous menu
			return m.goBack(), nil

		case "esc":
			return m.goBack(), nil

		case "up", "k":
			items := m.getMenuItems()
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				// Two-column navigation: up moves within column
				rows := (len(items) + 1) / 2
				if m.cursor > 0 && m.cursor < rows {
					m.cursor--
				} else if m.cursor >= rows && m.cursor > rows {
					m.cursor--
				} else if m.cursor == rows {
					m.cursor = rows - 1
				}
			} else {
				if m.cursor > 0 {
					m.cursor--
				}
			}

		case "down", "j":
			items := m.getMenuItems()
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				// Two-column navigation: down moves within column
				rows := (len(items) + 1) / 2
				if m.cursor < rows-1 {
					m.cursor++
				} else if m.cursor >= rows && m.cursor < len(items)-1 {
					m.cursor++
				} else if m.cursor == rows-1 && rows < len(items) {
					m.cursor = rows
				}
			} else {
				if m.cursor < len(items)-1 {
					m.cursor++
				}
			}

		case "left", "h":
			items := m.getMenuItems()
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				rows := (len(items) + 1) / 2
				if m.cursor >= rows {
					m.cursor -= rows
				}
			}

		case "right", "l":
			items := m.getMenuItems()
			if m.state == stateBrowseProjects || m.state == stateSelectProject {
				rows := (len(items) + 1) / 2
				if m.cursor < rows && m.cursor+rows < len(items) {
					m.cursor += rows
				}
			}

		case "enter", " ":
			return m.handleSelection()

		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			idx := int(msg.String()[0] - '1')
			items := m.getMenuItems()
			if idx < len(items) {
				m.cursor = idx
				return m.handleSelection()
			}
		}

	case cmdFinishedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Error: %v\n%s", msg.err, msg.output)
			m.messageType = "error"
		} else {
			m.message = msg.output
			m.messageType = "success"
		}
	}

	return m, nil
}

func (m model) goBack() model {
	switch m.state {
	case stateBrowseProjects, stateSetupProject, stateTools, stateSessions:
		m.state = stateMain
	case stateSessionActions:
		m.state = stateSessions
	case stateProjectActions:
		m.state = stateBrowseProjects
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
		if hostname == "dev.lan" {
			items = append(items, "Sessions")
		}
		return append(items, "Skip")

	case stateBrowseProjects:
		items := m.projects
		return append(items, "Back to menu")

	case stateProjectActions:
		sessionName := sanitizeTmuxName(m.selectedProject)
		hasSession := m.activeSessions[sessionName]
		var label string
		if hasSession {
			label = "Attach"
		} else {
			label = "Open"
		}
		items := []string{label, "Claude-logged"}
		if hasSession {
			items = append(items, "Kill session")
		}
		items = append(items, "Back")
		return items

	case stateSessions:
		items := m.sessionNames
		if len(items) == 0 {
			return []string{"Back"}
		}
		return append(items, "Back")

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
	case "Skip":
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

	case "Claude-logged":
		if exists {
			// Add new window in existing session
			exec.Command("tmux", "new-window", "-t", sessionName, "-c", m.selectedPath, "claude-logged").Run()
			if isInsideTmux() {
				return m, func() tea.Msg {
					exec.Command("tmux", "switch-client", "-t", sessionName).Run()
					return tea.Quit()
				}
			}
			return m, execAndQuit("tmux", "attach", "-t", sessionName)
		}
		// Create new session running claude-logged
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath, "claude-logged").Run()
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath, "claude-logged")

	case "Kill session":
		exec.Command("tmux", "kill-session", "-t", sessionName).Run()
		m.activeSessions = tmuxListSessions()
		m.message = fmt.Sprintf("Killed tmux session '%s'", sessionName)
		m.messageType = "success"
		return m.goBack(), nil

	case "Back":
		return m.goBack(), nil
	}
	return m, nil
}

func (m *model) loadSessions() {
	m.sessionNames = []string{}
	sessions := tmuxListSessions()
	for name := range sessions {
		m.sessionNames = append(m.sessionNames, name)
	}
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
		exec.Command("tmux", "kill-session", "-t", m.selectedSession).Run()
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
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath).Run()
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath)
	case "Launch claude-logged":
		if isInsideTmux() {
			return m, func() tea.Msg {
				exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", m.selectedPath, "claude-logged").Run()
				exec.Command("tmux", "switch-client", "-t", sessionName).Run()
				return tea.Quit()
			}
		}
		return m, execAndQuit("tmux", "new-session", "-s", sessionName, "-c", m.selectedPath, "claude-logged")
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

// Tmux helpers

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

	// Empty sessions message
	if m.state == stateSessions && len(m.sessionNames) == 0 {
		s.WriteString(dimStyle.Render("  No active tmux sessions"))
		s.WriteString("\n\n")
	}

	// Menu items
	items := m.getMenuItems()

	// Two-column layout for project lists
	if m.state == stateBrowseProjects || m.state == stateSelectProject {
		s.WriteString(m.renderTwoColumnMenu(items))
	} else {
		for i, item := range items {
			cursor := "  "
			style := normalStyle
			if i == m.cursor {
				cursor = cursorStyle.Render("> ")
				style = selectedStyle
			}

			num := dimStyle.Render(fmt.Sprintf("%d) ", i+1))
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
		return "Tmux Sessions"
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

func (m model) renderTwoColumnMenu(items []string) string {
	var s strings.Builder
	colWidth := 28

	// Calculate rows needed (items split across 2 columns)
	rows := (len(items) + 1) / 2

	for row := 0; row < rows; row++ {
		leftIdx := row
		rightIdx := row + rows

		// Left column
		if leftIdx < len(items) {
			cursor := "  "
			style := normalStyle
			if leftIdx == m.cursor {
				cursor = cursorStyle.Render("> ")
				style = selectedStyle
			}
			num := dimStyle.Render(fmt.Sprintf("%2d) ", leftIdx+1))

			indicator := ""
			indicatorLen := 0
			if m.state == stateBrowseProjects && items[leftIdx] != "Back to menu" {
				if m.activeSessions[sanitizeTmuxName(items[leftIdx])] {
					indicator = " " + lipgloss.NewStyle().Foreground(green).Render("●")
					indicatorLen = 2
				}
			}

			item := items[leftIdx]
			maxLen := colWidth - 6 - indicatorLen
			if len(item) > maxLen {
				item = item[:maxLen-3] + "..."
			}
			leftCol := cursor + num + style.Render(item) + indicator
			padding := colWidth - len(items[leftIdx]) - 6 - indicatorLen
			if padding < 0 {
				padding = 0
			}
			s.WriteString(leftCol + strings.Repeat(" ", padding))
		} else {
			s.WriteString(strings.Repeat(" ", colWidth))
		}

		// Right column
		if rightIdx < len(items) {
			cursor := "  "
			style := normalStyle
			if rightIdx == m.cursor {
				cursor = cursorStyle.Render("> ")
				style = selectedStyle
			}
			num := dimStyle.Render(fmt.Sprintf("%2d) ", rightIdx+1))

			indicator := ""
			indicatorLen := 0
			if m.state == stateBrowseProjects && items[rightIdx] != "Back to menu" {
				if m.activeSessions[sanitizeTmuxName(items[rightIdx])] {
					indicator = " " + lipgloss.NewStyle().Foreground(green).Render("●")
					indicatorLen = 2
				}
			}

			item := items[rightIdx]
			maxLen := colWidth - 6 - indicatorLen
			if len(item) > maxLen {
				item = item[:maxLen-3] + "..."
			}
			s.WriteString(cursor + num + style.Render(item) + indicator)
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
