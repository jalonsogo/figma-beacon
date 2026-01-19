package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type model struct {
	menuItems     []menuItem
	selectedIndex int
	width         int
	height        int
	profileStatus string
}

type menuItem struct {
	title       string
	description string
	warning     string
}

func initialModel() model {
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
		selectedIndex: 1,
		profileStatus: "⬥ No profile selected",
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			// For now, just placeholder
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
