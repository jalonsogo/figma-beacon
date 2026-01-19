# FIGMA BEACON TUI

A terminal user interface (TUI) application built with Go and Bubbletea, based on the Figma design.

## Features

- Gradient header bar with rainbow colors
- Menu navigation with keyboard controls
- Profile status indicator
- Gradient divider line
- Keyboard shortcuts display in footer

## Requirements

- Go 1.16 or higher

## Installation

```bash
# Clone or navigate to the project directory
cd tui-gradient

# Build the application
go build -o figma-beacon
```

## Usage

Run the application:

```bash
./figma-beacon
```

### Keyboard Controls

- **↑/k** - Move up in menu
- **↓/j** - Move down in menu
- **Enter** - Select menu item
- **Esc** - Back to home (reset selection)
- **Ctrl+C** or **q** - Quit the application

## Menu Items

1. **Generate Activity Report** - View your recent activity (requires profile selection)
2. **Manage Profiles** - Create, edit and manage your profiles
3. **Setup** - Configure API token and more

## Development

The TUI is built with:
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling and layout

To modify the application, edit `main.go` and rebuild.
