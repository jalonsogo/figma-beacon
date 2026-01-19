# FIGMA BEACON

A powerful terminal user interface (TUI) application for tracking and reporting your Figma activity. Monitor your team's projects, track file modifications, and generate beautiful activity reports directly from your terminal.

## Features

### Profile Management
- **Create monitoring profiles** - Define which Figma teams and projects you want to track
- **Multi-project tracking** - Select multiple projects within a team to monitor
- **Profile storage** - Profiles are saved as `.beacon` files in `~/.config/figma-beacon/profiles/`
- **Profile wizard** - Step-by-step guided flow for creating new profiles
- **Profile editing** - View, edit, and delete existing profiles
- **Default profile selection** - Set a default profile for quick report generation

### Activity Report Generation
- **Track file activity** - Automatically detect files created or modified within a time window
- **Flexible time windows** - Choose from multiple reporting periods:
  - Last Week (7 days)
  - Last Month (previous calendar month)
  - This Month to Date
  - Last 4 Weeks (28 days)
  - Last 30 Days
- **Smart activity detection** - Identifies both newly created files and modified existing files
- **Project-grouped reports** - Files are organized by their parent project
- **Markdown format** - Beautiful, readable reports with clickable Figma file links
- **Terminal rendering** - Reports are rendered in the terminal using Glamour with syntax highlighting
- **Export to file** - Save reports to `reports/` directory for sharing or archival

### Figma API Integration
- **User information fetching** - Automatically retrieve your Figma user ID, handle, and email
- **Team project discovery** - Browse and select projects from your Figma team
- **File metadata retrieval** - Access file names, modification dates, and creation timestamps
- **Version history tracking** - Query file version history to determine creation dates
- **Secure token storage** - API tokens are stored locally in configuration files

### Configuration Management
- **Persistent configuration** - Settings saved to `~/.config/figma-beacon/config.json`
- **API token management** - Securely store your Figma personal access token
- **Team and user settings** - Configure team ID and user information
- **Auto-load on startup** - Configuration and profiles load automatically when app starts

### User Interface
- **Responsive layout** - Adapts to your terminal size with footer always at bottom
- **Keyboard-driven** - Full keyboard navigation (no mouse required)
- **Real-time feedback** - Animated spinners and status messages during API calls
- **Profile status indicator** - Always know which profile is currently active
- **Clean gradient design** - Beautiful rainbow gradient headers and dividers
- **Visual step indicators** - Clear progress tracking in wizard flows

## Requirements

- Go 1.24.5 or higher
- Figma personal access token ([Get one here](https://www.figma.com/developers/api#access-tokens))
- Access to a Figma team

## Installation

```bash
# Clone the repository
git clone https://github.com/jalonsogo/figma-beacon.git
cd figma-beacon

# Build the application
go build -o figma-beacon

# Run it
./figma-beacon
```

## Quick Start

1. **First-time setup**
   - Run `./figma-beacon`
   - Navigate to "Setup" menu
   - Enter your Figma personal access token
   - Click "Gather" to fetch your user information automatically
   - Enter your Figma team ID

2. **Create a profile**
   - Navigate to "Manage Profiles"
   - Select "Create profile"
   - Follow the wizard to select projects you want to monitor
   - Give your profile a name and save

3. **Generate a report**
   - Navigate to "Generate Activity Report"
   - Select a time window (e.g., "Last Week")
   - View your activity report in the terminal
   - Reports are also saved to the `reports/` directory

## Keyboard Controls

### General Navigation
- **↑/k** - Move up in menu
- **↓/j** - Move down in menu
- **Enter** - Select menu item / Confirm action
- **Esc** - Go back / Cancel
- **Ctrl+C** or **q** - Quit the application

### Profile Wizard
- **Space** - Toggle selection (for multi-select lists)
- **Enter** - Confirm selection and proceed to next step
- **Esc** - Cancel wizard and return to profiles menu

### Text Input
- **Type** - Enter text
- **Backspace** - Delete character
- **Enter** - Confirm input

## Configuration Files

### Main Configuration
```
~/.config/figma-beacon/config.json
```
Stores:
- Figma API token
- User ID and handle
- Team ID
- User email

### Profile Storage
```
~/.config/figma-beacon/profiles/*.beacon
```
Each profile is stored as a separate JSON file containing:
- Profile name
- Team ID
- Selected projects (IDs and names)
- Creation timestamp
- Default profile flag

### Generated Reports
```
./reports/
```
Activity reports are exported as Markdown files to this directory.

## API Endpoints Used

Figma Beacon integrates with the following Figma REST API endpoints:
- `GET /v1/me` - Fetch authenticated user information
- `GET /v1/teams/{team_id}/projects` - List projects in a team
- `GET /v1/projects/{project_id}/files` - List files in a project
- `GET /v1/files/{file_key}` - Get file metadata
- `GET /v1/files/{file_key}/versions` - Get file version history

## Development

Built with modern Go libraries:
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling and layout
- [Glamour](https://github.com/charmbracelet/glamour) - Markdown rendering
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components

### Project Structure
- `main.go` - Single-file application containing all logic
- All state management uses the Elm architecture pattern (Model-Update-View)
- Async operations handled via Bubble Tea commands

### Building
```bash
go build -o figma-beacon
```

## Troubleshooting

**"Failed to fetch user info"**
- Verify your Figma token is correct
- Ensure your token has not expired
- Check your internet connection

**"No profile selected"**
- You must create at least one profile before generating reports
- Navigate to "Manage Profiles" → "Create profile"

**"No file activity found"**
- The selected profile's projects may not have any activity in the chosen time window
- Try selecting a different time window
- Verify the profile contains the correct projects

## License

MIT License - See LICENSE file for details

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
