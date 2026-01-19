package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screenState int

const (
	mainMenuScreen screenState = iota
	setupScreen
	formatSelectionScreen
)

type model struct {
	menuItems       []menuItem
	selectedIndex   int
	width           int
	height          int
	profileStatus   string
	currentScreen   screenState
	setupItems      []setupItem
	setupIndex      int
	figmaToken      string
	userID          string
	teamID          string
	reportFormat    string
	formatOptions   []string
	formatIndex     int
	textInput       textinput.Model
	editingIndex    int  // -1 means not editing, 0-2 means editing that field
	userHandle      string
	userEmail       string
	fetchingUser    bool
	userFetchError  string
}

type userInfoMsg struct {
	handle string
	id     string
	email  string
}

type userInfoErrMsg struct {
	err string
}

type config struct {
	FigmaToken   string `json:"figma_token"`
	UserID       string `json:"user_id"`
	TeamID       string `json:"team_id"`
	ReportFormat string `json:"report_format"`
	UserHandle   string `json:"user_handle"`
	UserEmail    string `json:"user_email"`
}

type setupItem struct {
	title string
	value string
}

type menuItem struct {
	title       string
	description string
	warning     string
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".config", "figma-beacon")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.json"), nil
}

func saveConfig(cfg config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func loadConfig() (config, error) {
	var cfg config

	configPath, err := getConfigPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Config file doesn't exist yet, return empty config
			return cfg, nil
		}
		return cfg, err
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 256
	ti.Width = 80
	ti.Prompt = ""

	// Load saved configuration
	cfg, _ := loadConfig()

	// Set default report format if not set
	if cfg.ReportFormat == "" {
		cfg.ReportFormat = "JSON"
	}

	return model{
		menuItems: []menuItem{
			{
				title:       "",
				description: "",
			},
			{
				title:       "Generate Activity Report",
				description: "View your recent activity",
				warning:     "● Profile!",
			},
			{
				title:       "Manage Profiles",
				description: "Create edit and manage your profiles",
			},
			{
				title:       "",
				description: "",
			},
			{
				title:       "Setup",
				description: "Configure API token and more",
			},
			{
				title:       "",
				description: "",
			},
		},
		selectedIndex:  1,
		profileStatus:  "⬥ No profile selected",
		currentScreen:  mainMenuScreen,
		setupIndex:     0,
		figmaToken:     cfg.FigmaToken,
		userID:         cfg.UserID,
		teamID:         cfg.TeamID,
		reportFormat:   cfg.ReportFormat,
		formatOptions:  []string{"JSON", "Markdown", "TXT", "HTML"},
		formatIndex:    0,
		textInput:      ti,
		editingIndex:   -1,
		userHandle:     cfg.UserHandle,
		userEmail:      cfg.UserEmail,
		fetchingUser:   false,
		userFetchError: "",
	}
}

func (m model) saveCurrentConfig() {
	cfg := config{
		FigmaToken:   m.figmaToken,
		UserID:       m.userID,
		TeamID:       m.teamID,
		ReportFormat: m.reportFormat,
		UserHandle:   m.userHandle,
		UserEmail:    m.userEmail,
	}
	saveConfig(cfg)
}

func (m model) Init() tea.Cmd {
	return nil
}

func fetchUserInfo(token string) tea.Cmd {
	return func() tea.Msg {
		if token == "" {
			return userInfoErrMsg{err: "No Figma token set"}
		}

		client := &http.Client{}
		req, err := http.NewRequest("GET", "https://api.figma.com/v1/me", nil)
		if err != nil {
			return userInfoErrMsg{err: err.Error()}
		}

		req.Header.Set("X-Figma-Token", token)

		resp, err := client.Do(req)
		if err != nil {
			return userInfoErrMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return userInfoErrMsg{err: fmt.Sprintf("API error: %s", string(body))}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return userInfoErrMsg{err: err.Error()}
		}

		var result struct {
			ID     string `json:"id"`
			Handle string `json:"handle"`
			Email  string `json:"email"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return userInfoErrMsg{err: err.Error()}
		}

		return userInfoMsg{
			id:     result.ID,
			handle: result.Handle,
			email:  result.Email,
		}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case userInfoMsg:
		m.userID = msg.id
		m.userHandle = msg.handle
		m.userEmail = msg.email
		m.fetchingUser = false
		m.userFetchError = ""
		m.saveCurrentConfig()
		return m, nil

	case userInfoErrMsg:
		m.fetchingUser = false
		m.userFetchError = msg.err
		return m, nil

	case tea.KeyMsg:
		// Handle format selection screen
		if m.currentScreen == formatSelectionScreen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Cancel and go back to setup screen
				m.currentScreen = setupScreen
				return m, nil
			case "up", "k":
				if m.formatIndex > 0 {
					m.formatIndex--
				}
			case "down", "j":
				if m.formatIndex < len(m.formatOptions)-1 {
					m.formatIndex++
				}
			case "enter":
				// Select format and go back to setup screen
				m.reportFormat = m.formatOptions[m.formatIndex]
				m.currentScreen = setupScreen
				m.saveCurrentConfig()
				return m, nil
			}
			return m, nil
		}

		// Handle setup screen with inline editing
		if m.currentScreen == setupScreen {
			// If we're editing a field
			if m.editingIndex >= 0 && m.editingIndex < 3 {
				switch msg.String() {
				case "ctrl+c":
					return m, tea.Quit
				case "esc":
					// Cancel editing and restore original value
					m.textInput.SetValue("")
					m.editingIndex = -1
					return m, nil
				case "enter":
					// Save value and exit editing mode
					value := m.textInput.Value()
					switch m.editingIndex {
					case 0:
						m.figmaToken = value
					case 1:
						m.userID = value
					case 2:
						m.teamID = value
					}
					m.textInput.SetValue("")
					m.editingIndex = -1
					m.saveCurrentConfig()
					return m, nil
				default:
					// Pass input to textinput
					m.textInput, cmd = m.textInput.Update(msg)
					return m, cmd
				}
			}

			// Not editing, handle navigation
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "esc":
				// Back to main menu
				m.currentScreen = mainMenuScreen
				m.selectedIndex = 1
				return m, nil
			case "up", "k":
				if m.setupIndex > 0 {
					m.setupIndex--
				}
			case "down", "j":
				if m.setupIndex < 4 { // 4 settings + back option = 5 items (0-4)
					m.setupIndex++
				}
			case "enter":
				switch m.setupIndex {
				case 0: // Set Figma Token
					m.editingIndex = 0
					m.textInput.SetValue(m.figmaToken)
					m.textInput.Focus()
				case 1: // Set User ID - Gather user info from API
					m.fetchingUser = true
					m.userFetchError = ""
					return m, fetchUserInfo(m.figmaToken)
				case 2: // Set Team ID
					m.editingIndex = 2
					m.textInput.SetValue(m.teamID)
					m.textInput.Focus()
				case 3: // Report format - show selection screen
					// Set initial selection to current format
					for i, f := range m.formatOptions {
						if f == m.reportFormat {
							m.formatIndex = i
							break
						}
					}
					m.currentScreen = formatSelectionScreen
				case 4: // Back
					m.currentScreen = mainMenuScreen
					m.selectedIndex = 1
				}
				return m, nil
			}
			return m, nil
		}

		// Handle main menu screen
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			// Back to home - reset to first menu item (skip empty line)
			m.selectedIndex = 1
		case "up", "k":
			if m.selectedIndex > 0 {
				m.selectedIndex--
				// Skip empty items
				if m.menuItems[m.selectedIndex].title == "" && m.selectedIndex > 0 {
					m.selectedIndex--
				}
			}
		case "down", "j":
			if m.selectedIndex < len(m.menuItems)-1 {
				m.selectedIndex++
				// Skip empty items
				if m.menuItems[m.selectedIndex].title == "" && m.selectedIndex < len(m.menuItems)-1 {
					m.selectedIndex++
				}
			}
		case "enter":
			// Handle menu selection
			if m.menuItems[m.selectedIndex].title == "Setup" {
				m.currentScreen = setupScreen
				m.setupIndex = 0
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

func (m model) View() string {
	// Return early if terminal size not yet received
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Route to appropriate view based on current screen
	switch m.currentScreen {
	case setupScreen:
		return m.viewSetupScreen()
	case formatSelectionScreen:
		return m.viewFormatSelection()
	default:
		return m.viewMainMenu()
	}
}

func (m model) viewMainMenu() string {

	// Define colors
	bgColor := lipgloss.Color("#020107")
	whiteColor := lipgloss.Color("#FFFFFF")
	defaultTextColor := lipgloss.Color("#C5C5C5")
	grayColor := lipgloss.Color("#7c7c7c")
	redColor := lipgloss.Color("#ea4536")
	cyanColor := lipgloss.Color("#00c7ff")
	dimWhiteColor := lipgloss.Color("rgba(255,255,255,0.4)")
	statusBgColor := lipgloss.Color("rgba(0,0,0,0.27)")

	// Gradient colors for header and divider
	gradientColors := []lipgloss.Color{
		lipgloss.Color("#4fc06b"), // green
		lipgloss.Color("#4aa9fb"), // blue
		lipgloss.Color("#7b48f9"), // purple
		lipgloss.Color("#ed7139"), // orange
		lipgloss.Color("#ea4536"), // red
	}

	// Create 3-line gradient header with text in the middle
	topGradientLine := createGradientBar(m.width, gradientColors)
	bottomGradientLine := createGradientBar(m.width, gradientColors)

	// Create middle line with gradient and overlay text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Create middle line with gradient and overlay text
	middleGradientLine := createGradientBarWithText(m.width, gradientColors, titleText, statusText, whiteColor, statusBgColor)

	// Menu items
	var menuStrings []string
	for i, item := range m.menuItems {
		if item.title == "" {
			menuStrings = append(menuStrings, "")
			continue
		}

		// Determine colors based on selection state
		var titleColor lipgloss.Color
		var isBold bool

		if i == m.selectedIndex {
			titleColor = whiteColor
			isBold = true
		} else {
			titleColor = defaultTextColor
			isBold = false
		}

		titleStyle := lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(isBold)

		descStyle := lipgloss.NewStyle().
			Foreground(grayColor)

		warningStyle := lipgloss.NewStyle().
			Foreground(redColor)

		titleText := item.title
		titleRendered := titleStyle.Render(item.title)

		if item.warning != "" {
			titleRendered = titleRendered + " " + warningStyle.Render(item.warning)
			titleText = titleText + " " + item.warning
		}

		rightSide := descStyle.Render(item.description)

		// Calculate spacing
		spacing := m.width - lipgloss.Width(titleText) - lipgloss.Width(item.description) - 4
		if spacing < 0 {
			spacing = 0
		}

		menuLine := lipgloss.JoinHorizontal(lipgloss.Top, titleRendered, strings.Repeat(" ", spacing), rightSide)

		menuStrings = append(menuStrings, menuLine)
	}

	menuSection := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(menuStrings, "\n"))

	// Create gradient divider
	divider := createGradientDivider(m.width, gradientColors)

	// Footer with keyboard shortcuts
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back to home")
	ctrlCStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("ctrl+c")
	ctrlCDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("quit")

	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc, "    ", ctrlCStyle, " ", ctrlCDesc)

	// Gradient dots
	dots := ""
	for _, color := range gradientColors {
		dots += lipgloss.NewStyle().Foreground(color).Render("⬤")
	}

	spacing := m.width - lipgloss.Width(leftShortcuts) - lipgloss.Width(dots) - 4
	if spacing < 0 {
		spacing = 0
	}

	footer := lipgloss.NewStyle().
		Background(bgColor).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, leftShortcuts, strings.Repeat(" ", spacing), dots))

	// Combine all sections
	var sections []string
	sections = append(sections, topGradientLine)
	sections = append(sections, middleGradientLine)
	sections = append(sections, bottomGradientLine)
	sections = append(sections, menuSection)
	sections = append(sections, divider)
	sections = append(sections, footer)

	return lipgloss.NewStyle().
		Background(bgColor).
		Height(m.height).
		Width(m.width).
		Render(strings.Join(sections, "\n"))
}

func (m model) viewSetupScreen() string {
	// Define colors
	bgColor := lipgloss.Color("#020107")
	whiteColor := lipgloss.Color("#FFFFFF")
	defaultTextColor := lipgloss.Color("#C5C5C5")
	grayColor := lipgloss.Color("#7c7c7c")
	cyanColor := lipgloss.Color("#00c7ff")
	dimWhiteColor := lipgloss.Color("rgba(255,255,255,0.4)")
	statusBgColor := lipgloss.Color("rgba(0,0,0,0.27)")

	// Gradient colors for header and divider
	gradientColors := []lipgloss.Color{
		lipgloss.Color("#4fc06b"), // green
		lipgloss.Color("#4aa9fb"), // blue
		lipgloss.Color("#7b48f9"), // purple
		lipgloss.Color("#ed7139"), // orange
		lipgloss.Color("#ea4536"), // red
	}

	// Create 3-line gradient header
	topGradientLine := createGradientBar(m.width, gradientColors)
	bottomGradientLine := createGradientBar(m.width, gradientColors)
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus
	middleGradientLine := createGradientBarWithText(m.width, gradientColors, titleText, statusText, whiteColor, statusBgColor)

	// Setup menu items
	var userIDValue string
	if m.fetchingUser {
		userIDValue = "Gathering..."
	} else if m.userFetchError != "" {
		userIDValue = "Error"
	} else if m.userID != "" {
		userIDValue = m.userID
	} else {
		userIDValue = "Gather"
	}

	setupItems := []struct {
		title string
		value string
	}{
		{"Set Figma Token", m.figmaToken},
		{"Set User ID", userIDValue},
		{"Set Team ID", m.teamID},
		{"Report format", m.reportFormat},
		{"← Back", "Back to main screen"},
	}

	// Build menu
	var menuStrings []string
	menuStrings = append(menuStrings, "") // Empty line at top

	// Display user info if available
	if m.userHandle != "" && m.userID != "" {
		userInfoStyle := lipgloss.NewStyle().
			Foreground(whiteColor)

		handleLine := fmt.Sprintf("  %s / (%s)", m.userHandle, m.userID)
		emailLine := fmt.Sprintf("  %s", m.userEmail)

		menuStrings = append(menuStrings, userInfoStyle.Render(handleLine))
		menuStrings = append(menuStrings, userInfoStyle.Render(emailLine))
		menuStrings = append(menuStrings, "")
	} else if m.userFetchError != "" {
		errorStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ea4536"))

		menuStrings = append(menuStrings, errorStyle.Render(fmt.Sprintf("  Error: %s", m.userFetchError)))
		menuStrings = append(menuStrings, "")
	}

	for i, item := range setupItems {
		// Add empty line before Back option
		if i == 4 {
			menuStrings = append(menuStrings, "")
		}

		var titleColor lipgloss.Color
		var isBold bool

		if i == m.setupIndex {
			titleColor = whiteColor
			isBold = true
		} else {
			titleColor = defaultTextColor
			isBold = false
		}

		titleStyle := lipgloss.NewStyle().
			Foreground(titleColor).
			Bold(isBold)

		leftSide := titleStyle.Render(item.title)

		// Determine the display text for right side
		rightText := item.value
		if rightText == "" && i < 3 {
			rightText = "Not set"
		}

		var rightSide string
		var rightWidth int

		// If this item is being edited, show input with gray background
		if m.editingIndex == i {
			inputContent := m.textInput.View()
			// Use the width of the input content or minimum width
			rightWidth = lipgloss.Width(inputContent)
			if rightWidth < lipgloss.Width(rightText) {
				rightWidth = lipgloss.Width(rightText)
			}

			inputStyle := lipgloss.NewStyle().
				Background(grayColor).
				Foreground(whiteColor)

			rightSide = inputStyle.Render(inputContent)
		} else {
			// Show value or "Not set"
			var descStyle lipgloss.Style

			// Special styling for "Gather" link
			if i == 1 && (rightText == "Gather" || rightText == "Gathering...") {
				descStyle = lipgloss.NewStyle().
					Foreground(cyanColor).
					Underline(true)
			} else if i == 1 && rightText == "Error" {
				descStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ea4536"))
			} else {
				descStyle = lipgloss.NewStyle().
					Foreground(grayColor)
			}

			rightSide = descStyle.Render(rightText)
			rightWidth = lipgloss.Width(rightText)
		}

		// Add arrow separator only when editing
		var middlePart string
		var arrowWidth int

		if m.editingIndex == i {
			arrowSep := " → "
			arrowStyle := lipgloss.NewStyle().Foreground(grayColor)
			arrowRendered := arrowStyle.Render(arrowSep)
			arrowWidth = lipgloss.Width(arrowSep)

			// Calculate spacing to align right
			spacing := m.width - lipgloss.Width(item.title) - arrowWidth - rightWidth - 4
			if spacing < 0 {
				spacing = 0
			}

			middlePart = arrowRendered + strings.Repeat(" ", spacing)
		} else {
			// No arrow, just spacing
			spacing := m.width - lipgloss.Width(item.title) - rightWidth - 4
			if spacing < 0 {
				spacing = 0
			}
			middlePart = strings.Repeat(" ", spacing)
		}

		menuLine := lipgloss.JoinHorizontal(lipgloss.Top, leftSide, middlePart, rightSide)
		menuStrings = append(menuStrings, menuLine)
	}

	menuStrings = append(menuStrings, "") // Empty line at bottom

	menuSection := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(menuStrings, "\n"))

	// Create gradient divider
	divider := createGradientDivider(m.width, gradientColors)

	// Footer
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back to menu")
	ctrlCStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("ctrl+c")
	ctrlCDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("quit")

	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc, "    ", ctrlCStyle, " ", ctrlCDesc)

	dots := ""
	for _, color := range gradientColors {
		dots += lipgloss.NewStyle().Foreground(color).Render("⬤")
	}

	spacing := m.width - lipgloss.Width(leftShortcuts) - lipgloss.Width(dots) - 4
	if spacing < 0 {
		spacing = 0
	}

	footer := lipgloss.NewStyle().
		Background(bgColor).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, leftShortcuts, strings.Repeat(" ", spacing), dots))

	// Combine all sections
	var sections []string
	sections = append(sections, topGradientLine)
	sections = append(sections, middleGradientLine)
	sections = append(sections, bottomGradientLine)
	sections = append(sections, menuSection)
	sections = append(sections, divider)
	sections = append(sections, footer)

	return lipgloss.NewStyle().
		Background(bgColor).
		Height(m.height).
		Width(m.width).
		Render(strings.Join(sections, "\n"))
}

func (m model) viewFormatSelection() string {
	// Define colors
	bgColor := lipgloss.Color("#020107")
	whiteColor := lipgloss.Color("#FFFFFF")
	defaultTextColor := lipgloss.Color("#C5C5C5")
	cyanColor := lipgloss.Color("#00c7ff")
	dimWhiteColor := lipgloss.Color("rgba(255,255,255,0.4)")
	statusBgColor := lipgloss.Color("rgba(0,0,0,0.27)")

	// Gradient colors for header and divider
	gradientColors := []lipgloss.Color{
		lipgloss.Color("#4fc06b"), // green
		lipgloss.Color("#4aa9fb"), // blue
		lipgloss.Color("#7b48f9"), // purple
		lipgloss.Color("#ed7139"), // orange
		lipgloss.Color("#ea4536"), // red
	}

	// Create 3-line gradient header
	topGradientLine := createGradientBar(m.width, gradientColors)
	bottomGradientLine := createGradientBar(m.width, gradientColors)
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus
	middleGradientLine := createGradientBarWithText(m.width, gradientColors, titleText, statusText, whiteColor, statusBgColor)

	// Build format selection list
	var menuStrings []string
	menuStrings = append(menuStrings, "")
	menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render("  Select Report Format:"))
	menuStrings = append(menuStrings, "")

	for i, format := range m.formatOptions {
		var formatColor lipgloss.Color
		var isBold bool
		var prefix string

		if i == m.formatIndex {
			formatColor = whiteColor
			isBold = true
			prefix = "  → "
		} else {
			formatColor = defaultTextColor
			isBold = false
			prefix = "    "
		}

		formatStyle := lipgloss.NewStyle().
			Foreground(formatColor).
			Bold(isBold)

		menuStrings = append(menuStrings, formatStyle.Render(prefix+format))
	}

	menuStrings = append(menuStrings, "")

	menuSection := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(menuStrings, "\n"))

	// Create gradient divider
	divider := createGradientDivider(m.width, gradientColors)

	// Footer
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("cancel")
	enterStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("enter")
	enterDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("select")

	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc, "    ", enterStyle, " ", enterDesc)

	dots := ""
	for _, color := range gradientColors {
		dots += lipgloss.NewStyle().Foreground(color).Render("⬤")
	}

	spacing := m.width - lipgloss.Width(leftShortcuts) - lipgloss.Width(dots) - 4
	if spacing < 0 {
		spacing = 0
	}

	footer := lipgloss.NewStyle().
		Background(bgColor).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, leftShortcuts, strings.Repeat(" ", spacing), dots))

	// Combine all sections
	var sections []string
	sections = append(sections, topGradientLine)
	sections = append(sections, middleGradientLine)
	sections = append(sections, bottomGradientLine)
	sections = append(sections, menuSection)
	sections = append(sections, divider)
	sections = append(sections, footer)

	return lipgloss.NewStyle().
		Background(bgColor).
		Height(m.height).
		Width(m.width).
		Render(strings.Join(sections, "\n"))
}

type rgb struct {
	r, g, b float64
}

func hexToRGB(hex string) rgb {
	// Remove # if present
	hex = strings.TrimPrefix(hex, "#")

	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)

	return rgb{
		r: float64(r),
		g: float64(g),
		b: float64(b),
	}
}

func rgbToHex(c rgb) string {
	return fmt.Sprintf("#%02x%02x%02x",
		uint8(c.r),
		uint8(c.g),
		uint8(c.b))
}

func interpolateColor(c1, c2 rgb, t float64) rgb {
	return rgb{
		r: c1.r + (c2.r-c1.r)*t,
		g: c1.g + (c2.g-c1.g)*t,
		b: c1.b + (c2.b-c1.b)*t,
	}
}

func createGradientBar(width int, colors []lipgloss.Color) string {
	if width <= 0 {
		return ""
	}

	// Convert lipgloss colors to RGB
	rgbColors := make([]rgb, len(colors))
	for i, color := range colors {
		rgbColors[i] = hexToRGB(string(color))
	}

	// Create smooth gradient by interpolating for each character position
	result := ""
	for i := 0; i < width; i++ {
		// Calculate position in gradient (0.0 to 1.0)
		position := float64(i) / float64(width-1)
		if width == 1 {
			position = 0
		}

		// Find which two colors to interpolate between
		scaledPos := position * float64(len(rgbColors)-1)
		idx1 := int(scaledPos)
		idx2 := idx1 + 1

		if idx2 >= len(rgbColors) {
			idx2 = len(rgbColors) - 1
			idx1 = idx2
		}

		// Calculate interpolation factor between the two colors
		t := scaledPos - float64(idx1)

		// Interpolate and render
		interpolated := interpolateColor(rgbColors[idx1], rgbColors[idx2], t)
		hexColor := rgbToHex(interpolated)

		result += lipgloss.NewStyle().
			Background(lipgloss.Color(hexColor)).
			Render(" ")
	}

	return result
}

func createGradientBarWithText(width int, colors []lipgloss.Color, titleText, statusText string, textColor, statusBg lipgloss.Color) string {
	if width <= 0 {
		return ""
	}

	// Convert lipgloss colors to RGB
	rgbColors := make([]rgb, len(colors))
	for i, color := range colors {
		rgbColors[i] = hexToRGB(string(color))
	}

	// Calculate text positioning
	statusWithPadding := " " + statusText + " "
	statusWidth := len([]rune(statusWithPadding))
	titleWithPadding := "  " + titleText
	titleWidth := len([]rune(titleWithPadding))

	// Calculate spacing between title and status
	spacing := width - titleWidth - statusWidth
	if spacing < 0 {
		spacing = 0
	}

	// Build the line character by character with gradient background
	result := ""

	for i := 0; i < width; i++ {
		// Calculate position in gradient (0.0 to 1.0)
		position := float64(i) / float64(width-1)
		if width == 1 {
			position = 0
		}

		// Find which two colors to interpolate between
		scaledPos := position * float64(len(rgbColors)-1)
		idx1 := int(scaledPos)
		idx2 := idx1 + 1

		if idx2 >= len(rgbColors) {
			idx2 = len(rgbColors) - 1
			idx1 = idx2
		}

		// Calculate interpolation factor between the two colors
		t := scaledPos - float64(idx1)

		// Interpolate background color
		interpolated := interpolateColor(rgbColors[idx1], rgbColors[idx2], t)
		hexColor := rgbToHex(interpolated)

		// Determine what character to render based on position
		var char string
		var useStatusBg bool

		if i < titleWidth {
			// Title area
			char = string([]rune(titleWithPadding)[i])
			useStatusBg = false
		} else if i < titleWidth+spacing {
			// Spacing area
			char = " "
			useStatusBg = false
		} else if i < titleWidth+spacing+statusWidth {
			// Status area
			statusIdx := i - titleWidth - spacing
			char = string([]rune(statusWithPadding)[statusIdx])
			useStatusBg = true
		} else {
			// Remaining space
			char = " "
			useStatusBg = false
		}

		// Apply styling
		if useStatusBg {
			result += lipgloss.NewStyle().
				Foreground(textColor).
				Background(statusBg).
				Render(char)
		} else {
			result += lipgloss.NewStyle().
				Foreground(textColor).
				Background(lipgloss.Color(hexColor)).
				Render(char)
		}
	}

	return result
}

func createGradientDivider(width int, colors []lipgloss.Color) string {
	if width <= 0 {
		return ""
	}

	// Convert lipgloss colors to RGB
	rgbColors := make([]rgb, len(colors))
	for i, color := range colors {
		rgbColors[i] = hexToRGB(string(color))
	}

	// Create smooth gradient divider by interpolating for each character position
	result := ""
	for i := 0; i < width; i++ {
		// Calculate position in gradient (0.0 to 1.0)
		position := float64(i) / float64(width-1)
		if width == 1 {
			position = 0
		}

		// Find which two colors to interpolate between
		scaledPos := position * float64(len(rgbColors)-1)
		idx1 := int(scaledPos)
		idx2 := idx1 + 1

		if idx2 >= len(rgbColors) {
			idx2 = len(rgbColors) - 1
			idx1 = idx2
		}

		// Calculate interpolation factor between the two colors
		t := scaledPos - float64(idx1)

		// Interpolate and render
		interpolated := interpolateColor(rgbColors[idx1], rgbColors[idx2], t)
		hexColor := rgbToHex(interpolated)

		result += lipgloss.NewStyle().
			Foreground(lipgloss.Color(hexColor)).
			Render("―")
	}

	return result
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
