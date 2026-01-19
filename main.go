package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/glamour"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screenState int

const (
	mainMenuScreen screenState = iota
	setupScreen
	manageProfilesScreen
	profileWizardScreen
	profilePreviewScreen
	reportConfigScreen
	reportGeneratingScreen
	reportViewScreen
)

type wizardStep int

const (
	wizardTeamID wizardStep = iota
	wizardProjects
	wizardSaveName
)

type loadingState int

const (
	notLoading loadingState = iota
	loadingProjects
)

// Profile data structures
type ProfileProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Profile struct {
	Name             string           `json:"name"`
	TeamID           string           `json:"team_id"`
	SelectedProjects []ProfileProject `json:"selected_projects"`
	CreatedAt        time.Time        `json:"created_at"`
	IsDefault        bool             `json:"is_default"`
}

type FigmaProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FigmaFile struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	ProjectID string // Not from API, added by us
}

// Report generator data structures
type timeMode string

const (
	timeModeLastWeek        timeMode = "last_week"
	timeModeLastMonth       timeMode = "last_month"
	timeModeThisMonthToDate timeMode = "this_month_to_date"
	timeModeLast4Weeks      timeMode = "last_4_weeks"
	timeModeLast30Days      timeMode = "last_30_days"
)

type ReportConfig struct {
	TimeMode  timeMode
	FileKeys  []string
	ProjectID string
}

type TimeWindow struct {
	Start time.Time
	End   time.Time
}

type FigmaFileMetadata struct {
	Key          string    `json:"key"`
	Name         string    `json:"name"`
	LastModified time.Time `json:"lastModified"`
	ThumbnailURL string    `json:"thumbnailUrl"`
}

type FigmaVersion struct {
	ID          string    `json:"id"`
	Created     time.Time `json:"created_at"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
	User        struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
	} `json:"user"`
}

type FigmaComment struct {
	ID        string    `json:"id"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	User      struct {
		ID     string `json:"id"`
		Handle string `json:"handle"`
	} `json:"user"`
}

type FileActivity struct {
	FileKey         string
	FileName        string
	ProjectName     string
	Versions        []FigmaVersion
	Comments        []FigmaComment
	LastModified    time.Time
	CreatedAt       time.Time
	MyChanges       bool // indicates if the user made changes in the time window
	CreatedInWindow bool // indicates if file was created in the time window
}

type ActivityReport struct {
	TimeWindow   TimeWindow
	UserID       string
	UserHandle   string
	Files        []FileActivity
	TotalFiles   int
	TotalChanges int
	GeneratedAt  time.Time
}

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
	textInput       textinput.Model
	editingIndex    int  // -1 means not editing, 0-2 means editing that field
	userHandle      string
	userEmail       string
	fetchingUser    bool
	userFetchError  string
	// Profile management fields
	profiles           []Profile
	activeProfile      *Profile
	previewProfile     *Profile
	wizardStep         wizardStep
	wizardTeamID       string
	wizardProjects     []FigmaProject
	wizardSelectedProj map[string]bool
	wizardProfileName  string
	wizardEditMode     bool // true if editing existing profile, false if creating new
	loadingState       loadingState
	loadingError       string
	loadingProgress    string
	listOffset         int
	listCursor         int
	// Delete confirmation
	showDeleteConfirm bool
	deleteProfileName string
	// Report generator fields
	reportConfig      ReportConfig
	reportTimeOptions []string
	reportTimeIndex   int
	reportProfileIndex int // Selected profile index for report
	generatingReport  bool
	reportingProfile  *Profile // Profile being used for current report generation
	activityReport    *ActivityReport
	reportError       string
	reportContent     string
	exportSuccess     string
	exportError       string
	spinnerFrame      int    // Current spinner frame
	spinnerChars      []string // Spinner characters
}

type userInfoMsg struct {
	handle string
	id     string
	email  string
}

type userInfoErrMsg struct {
	err string
}

// Profile message types
type projectsCompleteMsg struct {
	projects []FigmaProject
	count    int
}

type projectsErrMsg struct {
	err string
}

type profileSavedMsg struct {
	profileName string
}

type profilesLoadedMsg struct {
	profiles []Profile
}

// Report generator message types
type reportGeneratedMsg struct {
	report  *ActivityReport
	content string
}

type reportErrMsg struct {
	err string
}

type reportExportedMsg struct {
	filepath string
}

type reportExportErrMsg struct {
	err string
}

type tickMsg time.Time

type config struct {
	FigmaToken string `json:"figma_token"`
	UserID     string `json:"user_id"`
	TeamID     string `json:"team_id"`
	UserHandle string `json:"user_handle"`
	UserEmail  string `json:"user_email"`
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

// Profile storage functions
func getProfilesPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	profilesDir := filepath.Join(homeDir, ".config", "figma-beacon", "profiles")
	if err := os.MkdirAll(profilesDir, 0755); err != nil {
		return "", err
	}

	return profilesDir, nil
}

func saveProfile(profile Profile) error {
	profilesDir, err := getProfilesPath()
	if err != nil {
		return err
	}

	fileName := profile.Name + ".beacon"
	filePath := filepath.Join(profilesDir, fileName)

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

func loadProfile(name string) (Profile, error) {
	var profile Profile

	profilesDir, err := getProfilesPath()
	if err != nil {
		return profile, err
	}

	fileName := name + ".beacon"
	filePath := filepath.Join(profilesDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return profile, err
	}

	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, err
	}

	return profile, nil
}

func loadAllProfiles() ([]Profile, error) {
	profilesDir, err := getProfilesPath()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Profile{}, nil
		}
		return nil, err
	}

	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".beacon") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".beacon")
		profile, err := loadProfile(name)
		if err != nil {
			continue // Skip profiles that fail to load
		}

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

func setDefaultProfile(name string) error {
	profiles, err := loadAllProfiles()
	if err != nil {
		return err
	}

	// Clear all default flags
	for i := range profiles {
		profiles[i].IsDefault = false
		if err := saveProfile(profiles[i]); err != nil {
			return err
		}
	}

	// Set the new default
	for i := range profiles {
		if profiles[i].Name == name {
			profiles[i].IsDefault = true
			return saveProfile(profiles[i])
		}
	}

	return fmt.Errorf("profile not found: %s", name)
}

func deleteProfile(name string) error {
	profilesDir, err := getProfilesPath()
	if err != nil {
		return err
	}

	fileName := name + ".beacon"
	filePath := filepath.Join(profilesDir, fileName)

	return os.Remove(filePath)
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = ""
	ti.CharLimit = 256
	ti.Width = 80
	ti.Prompt = ""

	// Load saved configuration
	cfg, _ := loadConfig()

	// Load profiles
	profiles, _ := loadAllProfiles()

	// Find default profile
	var activeProfile *Profile
	for i := range profiles {
		if profiles[i].IsDefault {
			activeProfile = &profiles[i]
			break
		}
	}

	profileStatus := "⬥ No profile selected"
	if activeProfile != nil {
		profileStatus = "⬥ Profile: " + activeProfile.Name
	}

	// Build menu items with profiles integrated
	menuItems := []menuItem{
		{
			title:       "",
			description: "",
		},
		{
			title:       "Generate Activity Report",
			description: "View your recent activity",
			warning:     "",
		},
		{
			title:       "Manage Profiles",
			description: "Create edit and manage your profiles",
		},
	}

	// Sort profiles by creation date (most recent first) and add up to 3 under Manage Profiles
	sortedProfiles := make([]Profile, len(profiles))
	copy(sortedProfiles, profiles)
	sort.Slice(sortedProfiles, func(i, j int) bool {
		return sortedProfiles[i].CreatedAt.After(sortedProfiles[j].CreatedAt)
	})

	profileCount := 0
	for i := range sortedProfiles {
		if profileCount >= 3 {
			break
		}
		displayName := "  - " + sortedProfiles[i].Name
		if sortedProfiles[i].IsDefault {
			displayName += " (default)"
		}
		menuItems = append(menuItems, menuItem{
			title:       displayName,
			description: "",
		})
		profileCount++
	}

	menuItems = append(menuItems, menuItem{
		title:       "",
		description: "",
	})
	menuItems = append(menuItems, menuItem{
		title:       "Setup",
		description: "Configure API token and more",
	})
	menuItems = append(menuItems, menuItem{
		title:       "Exit",
		description: "Quit the application",
	})
	menuItems = append(menuItems, menuItem{
		title:       "",
		description: "",
	})

	return model{
		menuItems:           menuItems,
		selectedIndex:       1,
		profileStatus:       profileStatus,
		currentScreen:       mainMenuScreen,
		setupIndex:          0,
		figmaToken:          cfg.FigmaToken,
		userID:              cfg.UserID,
		teamID:              cfg.TeamID,
		textInput:           ti,
		editingIndex:        -1,
		userHandle:          cfg.UserHandle,
		userEmail:           cfg.UserEmail,
		fetchingUser:        false,
		userFetchError:      "",
		profiles:           profiles,
		activeProfile:      activeProfile,
		wizardStep:         wizardTeamID,
		wizardSelectedProj: make(map[string]bool),
		loadingState:       notLoading,
		listCursor:          0,
		listOffset:          0,
		showDeleteConfirm:   false,
		deleteProfileName:   "",
		reportTimeOptions:   []string{"Last Week", "Last Month", "This Month to Date", "Last 4 Weeks", "Last 30 Days"},
		reportTimeIndex:     0,
		reportProfileIndex:  0,
		generatingReport:    false,
		reportError:         "",
		spinnerFrame:        0,
		spinnerChars:        []string{"⬖", "⬗", "⬘", "⬙"},
	}
}

func (m model) saveCurrentConfig() {
	cfg := config{
		FigmaToken: m.figmaToken,
		UserID:     m.userID,
		TeamID:     m.teamID,
		UserHandle: m.userHandle,
		UserEmail:  m.userEmail,
	}
	saveConfig(cfg)
}

func (m model) Init() tea.Cmd {
	return nil
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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

// API functions for profile wizard
func fetchProjects(token, teamID string) tea.Cmd {
	return func() tea.Msg {
		if token == "" {
			return projectsErrMsg{err: "No Figma token set"}
		}

		if teamID == "" {
			return projectsErrMsg{err: "No team ID set"}
		}

		client := &http.Client{}
		url := fmt.Sprintf("https://api.figma.com/v1/teams/%s/projects", teamID)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return projectsErrMsg{err: err.Error()}
		}

		req.Header.Set("X-Figma-Token", token)

		resp, err := client.Do(req)
		if err != nil {
			return projectsErrMsg{err: err.Error()}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return projectsErrMsg{err: fmt.Sprintf("API error: %s", string(body))}
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return projectsErrMsg{err: err.Error()}
		}

		var result struct {
			Projects []FigmaProject `json:"projects"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return projectsErrMsg{err: err.Error()}
		}

		return projectsCompleteMsg{
			projects: result.Projects,
			count:    len(result.Projects),
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

	case projectsCompleteMsg:
		m.wizardProjects = msg.projects
		m.loadingState = notLoading
		m.loadingError = ""
		m.loadingProgress = fmt.Sprintf("Found %d projects", msg.count)
		m.listCursor = 0
		m.listOffset = 0
		return m, nil

	case projectsErrMsg:
		m.loadingState = notLoading
		m.loadingError = msg.err
		return m, nil

	case reportGeneratedMsg:
		m.generatingReport = false
		m.activityReport = msg.report
		m.reportContent = msg.content
		m.reportError = ""
		m.currentScreen = reportViewScreen

		// Restore profile status
		if m.activeProfile != nil {
			m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
		}
		m.reportingProfile = nil

		// Auto-export report
		profileName := "default"
		if m.activeProfile != nil {
			profileName = m.activeProfile.Name
		}
		return m, exportReport(msg.content, profileName)

	case reportErrMsg:
		m.generatingReport = false
		m.reportError = msg.err
		m.currentScreen = reportViewScreen

		// Restore profile status
		if m.activeProfile != nil {
			m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
		}
		m.reportingProfile = nil
		return m, nil

	case reportExportedMsg:
		m.exportSuccess = "Report saved to: " + msg.filepath
		m.exportError = ""
		return m, nil

	case reportExportErrMsg:
		m.exportSuccess = ""
		m.exportError = msg.err
		return m, nil

	case tickMsg:
		// Update spinner if generating report
		if m.generatingReport {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(m.spinnerChars)
			// Update profile status with spinner
			if m.reportingProfile != nil {
				m.profileStatus = m.spinnerChars[m.spinnerFrame] + " Profile: " + m.reportingProfile.Name
			}
			return m, tickCmd()
		}
		return m, nil

	case tea.KeyMsg:
		// Handle report config screen
		if m.currentScreen == reportConfigScreen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Back to main menu
				m.currentScreen = mainMenuScreen
				m.selectedIndex = 1
				return m, nil
			case "left", "h":
				// Navigate profiles
				if len(m.profiles) > 0 && m.reportProfileIndex > 0 {
					m.reportProfileIndex--
				}
			case "right", "l":
				// Navigate profiles
				if len(m.profiles) > 0 && m.reportProfileIndex < len(m.profiles)-1 {
					m.reportProfileIndex++
				}
			case "up", "k":
				// Navigate time options
				if m.reportTimeIndex > 0 {
					m.reportTimeIndex--
				}
			case "down", "j":
				// Navigate time options
				if m.reportTimeIndex < len(m.reportTimeOptions)-1 {
					m.reportTimeIndex++
				}
			case "enter":
				// Validate profile selected
				if len(m.profiles) == 0 {
					m.reportError = "No profiles available. Please create a profile first."
					return m, nil
				}

				// Generate report based on selected time mode
				var selectedMode timeMode
				switch m.reportTimeIndex {
				case 0:
					selectedMode = timeModeLastWeek
				case 1:
					selectedMode = timeModeLastMonth
				case 2:
					selectedMode = timeModeThisMonthToDate
				case 3:
					selectedMode = timeModeLast4Weeks
				case 4:
					selectedMode = timeModeLast30Days
				}

				m.reportConfig = ReportConfig{
					TimeMode: selectedMode,
				}

				// Use the selected profile
				selectedProfile := &m.profiles[m.reportProfileIndex]

				// Start report generation
				m.generatingReport = true
				m.reportingProfile = selectedProfile
				m.currentScreen = reportGeneratingScreen
				m.spinnerFrame = 0
				// Start both the report generation and the spinner
				return m, tea.Batch(
					generateReport(m.figmaToken, m.userID, m.userHandle, m.teamID, m.reportConfig, selectedProfile),
					tickCmd(),
				)
			}
			return m, nil
		}

		// Handle report view screen
		if m.currentScreen == reportGeneratingScreen || m.currentScreen == reportViewScreen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Back to main menu
				m.currentScreen = mainMenuScreen
				m.selectedIndex = 1
				m.generatingReport = false
				m.reportContent = ""
				m.reportError = ""
				m.exportSuccess = ""
				m.exportError = ""
				// Restore profile status
				if m.activeProfile != nil {
					m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
				}
				m.reportingProfile = nil
				return m, nil
			}
			return m, nil
		}

		// Handle profile preview screen
		if m.currentScreen == profilePreviewScreen {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Back to manage profiles
				m.currentScreen = manageProfilesScreen
				m.previewProfile = nil
				return m, nil
			case "e", "E":
				// Enter edit mode with current profile
				if m.previewProfile != nil {
					m.wizardEditMode = true
					m.wizardStep = wizardTeamID
					m.wizardTeamID = m.previewProfile.TeamID
					m.wizardProfileName = m.previewProfile.Name
					// Pre-select current projects
					m.wizardSelectedProj = make(map[string]bool)
					for _, proj := range m.previewProfile.SelectedProjects {
						m.wizardSelectedProj[proj.ID] = true
					}
					m.wizardProjects = nil
					m.loadingState = notLoading
					m.loadingError = ""
					m.listCursor = 0
					m.listOffset = 0
					m.currentScreen = profileWizardScreen
				}
				return m, nil
			case "d", "D":
				// Delete profile
				if m.previewProfile != nil {
					m.showDeleteConfirm = true
					m.deleteProfileName = m.previewProfile.Name
					m.currentScreen = manageProfilesScreen
					m.previewProfile = nil
				}
				return m, nil
			}
			return m, nil
		}

		// Handle profile wizard screen
		if m.currentScreen == profileWizardScreen {
			// If editing profile name in save step
			if m.wizardStep == wizardSaveName && m.editingIndex == 0 {
				switch msg.String() {
				case "ctrl+c":
					return m, tea.Quit
				case "esc":
					// Cancel editing
					m.textInput.SetValue("")
					m.editingIndex = -1
					return m, nil
				case "enter":
					// Save profile
					profileName := m.textInput.Value()
					m.textInput.SetValue("")
					m.editingIndex = -1

					// Validate profile name
					if profileName == "" {
						m.loadingError = "Profile name is required"
						return m, nil
					}

					// Check if profile name already exists (skip if in edit mode with same name)
					if !m.wizardEditMode || (m.wizardEditMode && profileName != m.previewProfile.Name) {
						for _, p := range m.profiles {
							if p.Name == profileName {
								m.loadingError = "Profile name already exists"
								return m, nil
							}
						}
					}

					// Build selected projects list
					var selectedProjects []ProfileProject
					for _, project := range m.wizardProjects {
						if m.wizardSelectedProj[project.ID] {
							selectedProjects = append(selectedProjects, ProfileProject{
								ID:   project.ID,
								Name: project.Name,
							})
						}
					}

					var profile Profile
					if m.wizardEditMode && m.previewProfile != nil {
						// Update existing profile
						// If name changed, delete old profile file
						if profileName != m.previewProfile.Name {
							deleteProfile(m.previewProfile.Name)
						}

						profile = Profile{
							Name:             profileName,
							TeamID:           m.wizardTeamID,
							SelectedProjects: selectedProjects,
							CreatedAt:        m.previewProfile.CreatedAt, // Preserve original creation time
							IsDefault:        m.previewProfile.IsDefault, // Preserve default status
						}
					} else {
						// Create new profile
						profile = Profile{
							Name:             profileName,
							TeamID:           m.wizardTeamID,
							SelectedProjects: selectedProjects,
							CreatedAt:        time.Now(),
							IsDefault:        len(m.profiles) == 0, // First profile is default
						}
					}

					// Save profile
					if err := saveProfile(profile); err != nil {
						m.loadingError = "Failed to save profile: " + err.Error()
						return m, nil
					}

					// Reload profiles
					profiles, _ := loadAllProfiles()
					m.profiles = profiles

					// Update active profile if it was being edited
					if m.wizardEditMode && m.activeProfile != nil && m.previewProfile != nil {
						if m.activeProfile.Name == m.previewProfile.Name {
							m.activeProfile = &profile
							m.profileStatus = "⬥ Profile: " + profile.Name
						}
					} else if profile.IsDefault {
						// Set as active if it's the default (new profile)
						m.activeProfile = &profile
						m.profileStatus = "⬥ Profile: " + profile.Name
					}

					// Reset wizard state
					m.wizardEditMode = false
					m.previewProfile = nil

					// Return to manage profiles
					m.currentScreen = manageProfilesScreen
					m.listCursor = 0
					return m, nil
				default:
					// Pass input to textinput
					m.textInput, cmd = m.textInput.Update(msg)
					return m, cmd
				}
			}

			// If editing Team ID
			if m.wizardStep == wizardTeamID && m.editingIndex == 0 {
				switch msg.String() {
				case "ctrl+c":
					return m, tea.Quit
				case "esc":
					// Cancel editing
					m.textInput.SetValue("")
					m.editingIndex = -1
					return m, nil
				case "enter":
					// Save value and move to next step
					m.wizardTeamID = m.textInput.Value()
					m.textInput.SetValue("")
					m.editingIndex = -1

					// Validate team ID is set
					if m.wizardTeamID == "" {
						m.loadingError = "Team ID is required"
						return m, nil
					}

					// Move to projects step and fetch projects
					m.wizardStep = wizardProjects
					m.loadingState = loadingProjects
					m.loadingError = ""
					m.listCursor = 0
					m.listOffset = 0
					return m, fetchProjects(m.figmaToken, m.wizardTeamID)
				default:
					// Pass input to textinput
					m.textInput, cmd = m.textInput.Update(msg)
					return m, cmd
				}
			}

			// Not editing
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Cancel wizard and go back to manage profiles
				m.currentScreen = manageProfilesScreen
				m.listCursor = 0
				m.wizardEditMode = false
				m.previewProfile = nil
				return m, nil
			case "up", "k":
				// Handle project list navigation
				if m.wizardStep == wizardProjects && len(m.wizardProjects) > 0 {
					if m.listCursor > 0 {
						m.listCursor--
						// Adjust offset for scrolling (fixed page size of 10)
						if m.listCursor < m.listOffset {
							m.listOffset = m.listCursor
						}
					}
				}
			case "down", "j":
				// Handle project list navigation
				if m.wizardStep == wizardProjects && len(m.wizardProjects) > 0 {
					if m.listCursor < len(m.wizardProjects)-1 {
						m.listCursor++
						// Adjust offset for scrolling (fixed page size of 10)
						if m.listCursor >= m.listOffset+10 {
							m.listOffset = m.listCursor - 10 + 1
						}
					}
				}
			case " ":
				// Toggle selection for projects
				if m.wizardStep == wizardProjects && len(m.wizardProjects) > 0 && m.listCursor < len(m.wizardProjects) {
					project := m.wizardProjects[m.listCursor]
					if m.wizardSelectedProj[project.ID] {
						delete(m.wizardSelectedProj, project.ID)
					} else {
						m.wizardSelectedProj[project.ID] = true
					}
				}
			case "enter":
				if m.wizardStep == wizardTeamID && m.editingIndex == -1 {
					// Start editing team ID
					m.editingIndex = 0
					m.textInput.SetValue(m.wizardTeamID)
					// Adjust text input width to fit terminal
					inputWidth := m.width - 8 // Account for padding and margins
					if inputWidth > 80 {
						inputWidth = 80
					}
					if inputWidth < 20 {
						inputWidth = 20
					}
					m.textInput.Width = inputWidth
					m.textInput.Focus()
					return m, nil
				} else if m.wizardStep == wizardProjects {
					// Validate at least one project selected
					if len(m.wizardSelectedProj) == 0 {
						m.loadingError = "Please select at least one project"
						return m, nil
					}

					// Collect selected project IDs
					var selectedProjectIDs []string
					for _, project := range m.wizardProjects {
						if m.wizardSelectedProj[project.ID] {
							selectedProjectIDs = append(selectedProjectIDs, project.ID)
						}
					}

					// Move directly to save name step
					m.wizardStep = wizardSaveName
					m.loadingError = ""
					m.editingIndex = -1
					return m, nil
				} else if m.wizardStep == wizardSaveName && m.editingIndex == -1 {
					// Start editing profile name
					m.editingIndex = 0
					m.textInput.SetValue(m.wizardProfileName)
					// Adjust text input width to fit terminal
					inputWidth := m.width - 8 // Account for padding and margins
					if inputWidth > 80 {
						inputWidth = 80
					}
					if inputWidth < 20 {
						inputWidth = 20
					}
					m.textInput.Width = inputWidth
					m.textInput.Focus()
					return m, nil
				}
			}
			return m, nil
		}

		// Handle manage profiles screen
		if m.currentScreen == manageProfilesScreen {
			// Handle delete confirmation
			if m.showDeleteConfirm {
				switch msg.String() {
				case "y", "Y":
					// Confirm delete
					err := deleteProfile(m.deleteProfileName)
					if err == nil {
						// Reload profiles
						profiles, _ := loadAllProfiles()
						m.profiles = profiles

						// If deleted profile was active, clear it
						if m.activeProfile != nil && m.activeProfile.Name == m.deleteProfileName {
							m.activeProfile = nil
							m.profileStatus = "⬥ No profile selected"
							// Set first remaining profile as default if any exist
							if len(m.profiles) > 0 {
								setDefaultProfile(m.profiles[0].Name)
								profiles, _ = loadAllProfiles()
								m.profiles = profiles
								for i := range m.profiles {
									if m.profiles[i].IsDefault {
										m.activeProfile = &m.profiles[i]
										m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
										break
									}
								}
							}
						}

						// Reset cursor if needed
						if m.listCursor > len(m.profiles) {
							m.listCursor = len(m.profiles)
						}
					}
					m.showDeleteConfirm = false
					m.deleteProfileName = ""
					return m, nil
				case "n", "N", "esc":
					// Cancel delete
					m.showDeleteConfirm = false
					m.deleteProfileName = ""
					return m, nil
				}
				return m, nil
			}

			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				// Back to main menu
				m.currentScreen = mainMenuScreen
				m.selectedIndex = 1
				return m, nil
			case "up", "k":
				if m.listCursor > 0 {
					m.listCursor--
				}
			case "down", "j":
				maxCursor := 1 + len(m.profiles) // Create + profiles + back
				if m.listCursor < maxCursor {
					m.listCursor++
				}
			case "backspace":
				// Delete profile if one is selected
				if m.listCursor > 0 && m.listCursor <= len(m.profiles) {
					profileIndex := m.listCursor - 1
					m.deleteProfileName = m.profiles[profileIndex].Name
					m.showDeleteConfirm = true
				}
				return m, nil
			case "d", "D":
				// Make selected profile default
				if m.listCursor > 0 && m.listCursor <= len(m.profiles) {
					profileIndex := m.listCursor - 1
					selectedProfile := m.profiles[profileIndex]
					setDefaultProfile(selectedProfile.Name)
					// Reload profiles
					profiles, _ := loadAllProfiles()
					m.profiles = profiles
					for i := range m.profiles {
						if m.profiles[i].IsDefault {
							m.activeProfile = &m.profiles[i]
							m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
							break
						}
					}
				}
				return m, nil
			case "enter":
				if m.listCursor == 0 {
					// Create new profile - enter wizard
					m.currentScreen = profileWizardScreen
					m.wizardStep = wizardTeamID
					m.wizardTeamID = m.teamID
					m.wizardSelectedProj = make(map[string]bool)
					m.wizardProfileName = ""
					m.wizardEditMode = false
					m.loadingState = notLoading
					m.loadingError = ""
					m.loadingProgress = ""
					m.listCursor = 0
				} else if m.listCursor == 1+len(m.profiles) {
					// Back to main menu
					m.currentScreen = mainMenuScreen
					m.selectedIndex = 1
				} else if m.listCursor > 0 && m.listCursor <= len(m.profiles) {
					// Preview profile
					profileIndex := m.listCursor - 1
					m.previewProfile = &m.profiles[profileIndex]
					m.currentScreen = profilePreviewScreen
				}
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
					// Adjust text input width to fit terminal
					inputWidth := m.width - 8 // Account for padding and margins
					if inputWidth > 80 {
						inputWidth = 80
					}
					if inputWidth < 20 {
						inputWidth = 20
					}
					m.textInput.Width = inputWidth
					m.textInput.Focus()
				case 1: // Set User ID - Gather user info from API
					m.fetchingUser = true
					m.userFetchError = ""
					return m, fetchUserInfo(m.figmaToken)
				case 2: // Set Team ID
					m.editingIndex = 2
					m.textInput.SetValue(m.teamID)
					// Adjust text input width to fit terminal
					inputWidth := m.width - 8 // Account for padding and margins
					if inputWidth > 80 {
						inputWidth = 80
					}
					if inputWidth < 20 {
						inputWidth = 20
					}
					m.textInput.Width = inputWidth
					m.textInput.Focus()
				case 3: // Back
					m.currentScreen = mainMenuScreen
					m.selectedIndex = 1
				}
				return m, nil
			}
			return m, nil
		}

		// Handle main menu screen
		if m.currentScreen == mainMenuScreen {
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
				selectedTitle := m.menuItems[m.selectedIndex].title

				if selectedTitle == "Generate Activity Report" {
					m.currentScreen = reportConfigScreen
					m.reportTimeIndex = 0
					// Set profile index to active profile or default to 0
					m.reportProfileIndex = 0
					if m.activeProfile != nil {
						for i, profile := range m.profiles {
							if profile.Name == m.activeProfile.Name {
								m.reportProfileIndex = i
								break
							}
						}
					}
				} else if selectedTitle == "Setup" {
					m.currentScreen = setupScreen
					m.setupIndex = 0
				} else if selectedTitle == "Exit" {
					return m, tea.Quit
				} else if selectedTitle == "Manage Profiles" {
					m.currentScreen = manageProfilesScreen
					m.listCursor = 0
				} else if strings.HasPrefix(selectedTitle, "  - ") {
					// Profile selected - extract profile name and set as active
					profileName := strings.TrimPrefix(selectedTitle, "  - ")
					profileName = strings.TrimSuffix(profileName, " (default)")

					// Set as active profile
					setDefaultProfile(profileName)
					// Reload profiles
					profiles, _ := loadAllProfiles()
					m.profiles = profiles
					for i := range m.profiles {
						if m.profiles[i].IsDefault {
							m.activeProfile = &m.profiles[i]
							m.profileStatus = "⬥ Profile: " + m.activeProfile.Name
							break
						}
					}
					// Rebuild menu items
					oldWidth := m.width
					oldHeight := m.height
					m = initialModel()
					m.width = oldWidth
					m.height = oldHeight
				}
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
	case manageProfilesScreen:
		return m.viewManageProfiles()
	case profileWizardScreen:
		return m.viewProfileWizard()
	case profilePreviewScreen:
		return m.viewProfilePreview()
	case reportConfigScreen:
		return m.viewReportConfig()
	case reportGeneratingScreen, reportViewScreen:
		return m.viewReportView()
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

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

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Render(strings.Join(menuStrings, "\n"))

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

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
		if i == 3 {
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

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Render(strings.Join(menuStrings, "\n"))

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
}

func (m model) viewManageProfiles() string {
	// Define colors
	bgColor := lipgloss.Color("#020107")
	whiteColor := lipgloss.Color("#FFFFFF")
	defaultTextColor := lipgloss.Color("#C5C5C5")
	greenColor := lipgloss.Color("#4fc06b")
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Build profile list
	var menuStrings []string
	menuStrings = append(menuStrings, "")
	menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render("  Manage Profiles"))
	menuStrings = append(menuStrings, "")

	// Add "Create new profile" option at index 0 (styled as button)
	cursor := 0
	var buttonText string

	if m.listCursor == cursor {
		// Active/hover - show as button with background
		createButtonStyle := lipgloss.NewStyle().
			Foreground(whiteColor).
			Background(lipgloss.Color("#4AA9FB")).
			Bold(true).
			Padding(0, 2)
		buttonText = "  " + createButtonStyle.Render("+ Create profile")
	} else {
		// Non-active - no background, just text
		createTextStyle := lipgloss.NewStyle().
			Foreground(defaultTextColor).
			Bold(false)
		buttonText = createTextStyle.Render("    + Create profile")
	}

	menuStrings = append(menuStrings, buttonText)
	cursor++

	// Show existing profiles or "No profiles" message
	if len(m.profiles) == 0 {
		menuStrings = append(menuStrings, "")
		noProfileStyle := lipgloss.NewStyle().
			Foreground(dimWhiteColor)
		menuStrings = append(menuStrings, noProfileStyle.Render("    No profiles created yet"))
	} else {
		menuStrings = append(menuStrings, "")
		for i, profile := range m.profiles {
			var profileColor lipgloss.Color
			var profileBold bool
			var profilePrefix string

			if m.listCursor == cursor {
				profileColor = whiteColor
				profileBold = true
				profilePrefix = "  → "
			} else {
				profileColor = defaultTextColor
				profileBold = false
				profilePrefix = "    "
			}

			profileStyle := lipgloss.NewStyle().
				Foreground(profileColor).
				Bold(profileBold)

			displayName := profile.Name
			if profile.IsDefault {
				displayName += " (default)"
				if m.listCursor != cursor {
					profileStyle = profileStyle.Foreground(greenColor)
				}
			}

			menuStrings = append(menuStrings, profileStyle.Render(profilePrefix+displayName))
			cursor++

			// Show profile details if selected
			if m.listCursor == i+1 {
				detailStyle := lipgloss.NewStyle().Foreground(dimWhiteColor)
				menuStrings = append(menuStrings, detailStyle.Render(fmt.Sprintf("      Projects: %d", len(profile.SelectedProjects))))
			}
		}
	}

	menuStrings = append(menuStrings, "")
	menuStrings = append(menuStrings, "")

	// Add "Back" option
	var backColor lipgloss.Color
	var backBold bool
	var backPrefix string

	if m.listCursor == cursor {
		backColor = whiteColor
		backBold = true
		backPrefix = "  ← "
	} else {
		backColor = defaultTextColor
		backBold = false
		backPrefix = "    "
	}

	backStyle := lipgloss.NewStyle().
		Foreground(backColor).
		Bold(backBold)

	menuStrings = append(menuStrings, backStyle.Render(backPrefix+"Back"))

	menuSection := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(menuStrings, "\n"))

	// Show delete confirmation dialog if needed
	if m.showDeleteConfirm {
		confirmBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#ea4536")).
			Padding(1, 2).
			Background(bgColor).
			Foreground(whiteColor)

		confirmTitle := lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Bold(true).Render("Delete Profile?")
		confirmMsg := lipgloss.NewStyle().Foreground(defaultTextColor).Render(fmt.Sprintf("\nAre you sure you want to delete '%s'?\n\n", m.deleteProfileName))
		confirmOptions := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("Y = Yes    N = No")

		confirmContent := confirmTitle + confirmMsg + confirmOptions
		confirmDialog := confirmBox.Render(confirmContent)

		// Center the dialog
		dialogWidth := lipgloss.Width(confirmDialog)
		dialogHeight := lipgloss.Height(confirmDialog)
		verticalPadding := (m.height - dialogHeight) / 2
		horizontalPadding := (m.width - dialogWidth) / 2

		if verticalPadding < 0 {
			verticalPadding = 0
		}
		if horizontalPadding < 0 {
			horizontalPadding = 0
		}

		// Overlay the dialog on the menu
		menuLines := strings.Split(menuSection, "\n")
		dialogLines := strings.Split(confirmDialog, "\n")

		for i, line := range dialogLines {
			lineIdx := verticalPadding + i
			if lineIdx >= 0 && lineIdx < len(menuLines) {
				padding := strings.Repeat(" ", horizontalPadding)
				menuLines[lineIdx] = padding + line
			}
		}

		menuSection = strings.Join(menuLines, "\n")
	}

	// Convert menuSection to final content
	content := menuSection

	// Footer
	var leftShortcuts string
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back to menu")
	enterStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("enter")
	enterDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("preview")

	if m.showDeleteConfirm {
		// Show Y/N shortcuts when delete confirmation is active
		yStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("y")
		yDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("yes")
		nStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("n")
		nDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("no")

		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top, yStyle, " ", yDesc, "    ", nStyle, " ", nDesc)
	} else if m.listCursor > 0 && m.listCursor <= len(m.profiles) {
		// Show options when a profile is selected
		backspaceStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("backspace")
		backspaceDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("delete")
		dStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("d")
		dDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("make default")

		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc, "    ", enterStyle, " ", enterDesc, "    ", backspaceStyle, " ", backspaceDesc, "    ", dStyle, " ", dDesc)
	} else {
		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc, "    ", enterStyle, " ", enterDesc)
	}

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
}

func (m model) viewProfileWizard() string {
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Build wizard screen based on current step
	var menuStrings []string
	menuStrings = append(menuStrings, "")
	wizardTitle := "  Create Profile"
	if m.wizardEditMode {
		wizardTitle = "  Edit Profile"
	}
	menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render(wizardTitle))
	menuStrings = append(menuStrings, "")

	// Step indicators with chevrons
	var stepParts []string
	greenColor := lipgloss.Color("#4fc06b")
	chevronStyle := lipgloss.NewStyle().Foreground(dimWhiteColor)

	// Step 1: Team ID
	step1Style := lipgloss.NewStyle().Foreground(dimWhiteColor)
	step1Indicator := "○"
	if m.wizardStep == wizardTeamID {
		step1Indicator = "●"
		step1Style = lipgloss.NewStyle().Foreground(whiteColor).Bold(true)
	} else if m.wizardStep > wizardTeamID {
		step1Indicator = "✓"
		step1Style = lipgloss.NewStyle().Foreground(greenColor)
	}
	stepParts = append(stepParts, step1Style.Render(step1Indicator+" Team ID"))
	stepParts = append(stepParts, chevronStyle.Render(" ❯ "))

	// Step 2: Projects
	step2Style := lipgloss.NewStyle().Foreground(dimWhiteColor)
	step2Indicator := "○"
	if m.wizardStep == wizardProjects {
		step2Indicator = "●"
		step2Style = lipgloss.NewStyle().Foreground(whiteColor).Bold(true)
	} else if m.wizardStep > wizardProjects {
		step2Indicator = "✓"
		step2Style = lipgloss.NewStyle().Foreground(greenColor)
	}
	stepParts = append(stepParts, step2Style.Render(step2Indicator+" Projects"))
	stepParts = append(stepParts, chevronStyle.Render(" ❯ "))

	// Step 3: Save
	step3Style := lipgloss.NewStyle().Foreground(dimWhiteColor)
	step3Indicator := "○"
	if m.wizardStep == wizardSaveName {
		step3Indicator = "●"
		step3Style = lipgloss.NewStyle().Foreground(whiteColor).Bold(true)
	}
	stepParts = append(stepParts, step3Style.Render(step3Indicator+" Save"))

	stepsLine := "  " + strings.Join(stepParts, "")
	menuStrings = append(menuStrings, stepsLine)
	menuStrings = append(menuStrings, "")

	// Render content based on current step
	switch m.wizardStep {
	case wizardTeamID:
		menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(defaultTextColor).Render("  Team ID:"))
		menuStrings = append(menuStrings, "")

		// Show input field
		if m.editingIndex == 0 {
			inputContent := m.textInput.View()
			inputStyle := lipgloss.NewStyle().
				Background(grayColor).
				Foreground(whiteColor)
			menuStrings = append(menuStrings, "  "+inputStyle.Render(inputContent))
		} else {
			valueStyle := lipgloss.NewStyle().Foreground(whiteColor)
			displayValue := m.wizardTeamID
			if displayValue == "" {
				displayValue = "Not set"
				valueStyle = valueStyle.Foreground(dimWhiteColor)
			}
			menuStrings = append(menuStrings, "  "+valueStyle.Render(displayValue))
		}

		menuStrings = append(menuStrings, "")

	case wizardProjects:
		if m.loadingState == loadingProjects {
			menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(cyanColor).Render("  Loading projects..."))
			if m.loadingProgress != "" {
				menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  "+m.loadingProgress))
			}
		} else if m.loadingError != "" {
			menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Render("  Error: "+m.loadingError))
		} else if len(m.wizardProjects) == 0 {
			menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  No projects found"))
		} else {
			// Show project list with multi-select and pagination
			selectedCount := len(m.wizardSelectedProj)
			headerText := fmt.Sprintf("  Select projects (%d selected):", selectedCount)
			menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(defaultTextColor).Render(headerText))
			menuStrings = append(menuStrings, "")

			// Fixed page size of 10 items
			visibleLines := 10

			// Calculate pagination
			totalItems := len(m.wizardProjects)
			startIdx := m.listOffset
			endIdx := startIdx + visibleLines
			if endIdx > totalItems {
				endIdx = totalItems
			}

			// Render visible project list
			for i := startIdx; i < endIdx; i++ {
				project := m.wizardProjects[i]
				var marker string
				var itemStyle lipgloss.Style

				// Check if selected
				isSelected := m.wizardSelectedProj[project.ID]

				// Determine marker and style
				if isSelected {
					marker = "➤ "
					itemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4fc06b")) // green
				} else {
					marker = "  "
					itemStyle = lipgloss.NewStyle().Foreground(defaultTextColor)
				}

				// Highlight cursor position
				if i == m.listCursor {
					itemStyle = itemStyle.Bold(true).Foreground(whiteColor)
				}

				line := "  " + marker + project.Name
				menuStrings = append(menuStrings, itemStyle.Render(line))
			}

			menuStrings = append(menuStrings, "")

			// Show pagination indicator if needed
			if totalItems > visibleLines {
				pageInfo := fmt.Sprintf("  [%d-%d of %d]", startIdx+1, endIdx, totalItems)
				menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render(pageInfo))
			}
		}

	case wizardSaveName:
		menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(defaultTextColor).Render("  Profile name:"))
		menuStrings = append(menuStrings, "")

		// Show input field
		if m.editingIndex == 0 {
			inputContent := m.textInput.View()
			inputStyle := lipgloss.NewStyle().
				Background(grayColor).
				Foreground(whiteColor)
			menuStrings = append(menuStrings, "  "+inputStyle.Render(inputContent))
		} else {
			valueStyle := lipgloss.NewStyle().Foreground(whiteColor)
			displayValue := m.wizardProfileName
			if displayValue == "" {
				displayValue = "Not set"
				valueStyle = valueStyle.Foreground(dimWhiteColor)
			}
			menuStrings = append(menuStrings, "  "+valueStyle.Render(displayValue))
		}

		menuStrings = append(menuStrings, "")

		if m.loadingError != "" {
			menuStrings = append(menuStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Render("  Error: "+m.loadingError))
			menuStrings = append(menuStrings, "")
		}
	}

	menuStrings = append(menuStrings, "")

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Render(strings.Join(menuStrings, "\n"))

	// Footer with dynamic shortcuts based on wizard step
	var leftShortcuts string
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("cancel")
	enterStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("enter")

	if m.wizardStep == wizardProjects  {
		// Show space and enter shortcuts for list screens
		spaceStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("space")
		spaceDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("toggle")
		enterDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("continue")

		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top,
			escStyle, " ", escDesc, "    ",
			spaceStyle, " ", spaceDesc, "    ",
			enterStyle, " ", enterDesc)
	} else if m.wizardStep == wizardTeamID || m.wizardStep == wizardSaveName {
		// Show enter shortcut for input screens
		enterDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("edit")

		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top,
			escStyle, " ", escDesc, "    ",
			enterStyle, " ", enterDesc)
	} else {
		// Show only esc for other screens
		leftShortcuts = lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc)
	}

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
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

func (m model) viewProfilePreview() string {
	if m.previewProfile == nil {
		return "No profile selected"
	}

	// Define colors
	bgColor := lipgloss.Color("#020107")
	whiteColor := lipgloss.Color("#FFFFFF")
	defaultTextColor := lipgloss.Color("#C5C5C5")
	greenColor := lipgloss.Color("#4fc06b")
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Build profile preview content
	var contentStrings []string
	contentStrings = append(contentStrings, "")

	// Profile name header
	profileNameStyle := lipgloss.NewStyle().Foreground(whiteColor).Bold(true)
	contentStrings = append(contentStrings, profileNameStyle.Render("  "+m.previewProfile.Name))

	if m.previewProfile.IsDefault {
		defaultBadge := lipgloss.NewStyle().Foreground(greenColor).Render("  (default)")
		contentStrings = append(contentStrings, defaultBadge)
	}

	contentStrings = append(contentStrings, "")

	// Team ID
	labelStyle := lipgloss.NewStyle().Foreground(dimWhiteColor)
	valueStyle := lipgloss.NewStyle().Foreground(defaultTextColor)
	contentStrings = append(contentStrings, labelStyle.Render("  Team ID: ")+valueStyle.Render(m.previewProfile.TeamID))
	contentStrings = append(contentStrings, "")

	// Display projects list
	contentStrings = append(contentStrings, labelStyle.Render("  Projects:"))
	darkGreyColor := lipgloss.Color("#666666")
	idStyle := lipgloss.NewStyle().Foreground(darkGreyColor)

	for i, project := range m.previewProfile.SelectedProjects {
		// Determine if this is the last project
		isLastProject := (i == len(m.previewProfile.SelectedProjects)-1)
		var projectPrefix string

		if isLastProject {
			projectPrefix = "  └╼ "
		} else {
			projectPrefix = "  ├╼ "
		}

		// Display project name and ID: "├╼ {name} (id)"
		projectLine := valueStyle.Render(projectPrefix+project.Name) + " " + idStyle.Render("("+project.ID+")")
		contentStrings = append(contentStrings, projectLine)
	}

	contentStrings = append(contentStrings, "")
	contentStrings = append(contentStrings, "")

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(contentStrings, "\n"))


	// Footer
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back")
	editStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("e")
	editDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("edit")
	deleteStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("d")
	deleteDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("delete")

	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top,
		escStyle, " ", escDesc, "    ",
		editStyle, " ", editDesc, "    ",
		deleteStyle, " ", deleteDesc)

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
}

func resolveTimeWindow(config ReportConfig) TimeWindow {
	now := time.Now()
	var start, end time.Time

	switch config.TimeMode {
	case timeModeLastWeek:
		// Last 7 days
		start = now.AddDate(0, 0, -7)
		end = now

	case timeModeLastMonth:
		// Previous calendar month
		firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		firstOfLastMonth := firstOfThisMonth.AddDate(0, -1, 0)
		start = firstOfLastMonth
		end = firstOfThisMonth.Add(-time.Second) // Last second of previous month

	case timeModeThisMonthToDate:
		// From first day of current month to now
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = now

	case timeModeLast4Weeks:
		// Last 28 days
		start = now.AddDate(0, 0, -28)
		end = now

	case timeModeLast30Days:
		// Last 30 days
		start = now.AddDate(0, 0, -30)
		end = now
	}

	return TimeWindow{
		Start: start,
		End:   end,
	}
}

func generateReport(token, userID, userHandle, teamID string, config ReportConfig, profile *Profile) tea.Cmd {
	return func() tea.Msg {
		window := resolveTimeWindow(config)

		// Ensure profile is selected
		if profile == nil {
			return reportErrMsg{err: "No profile selected. Please select a profile or create one in Manage Profiles."}
		}

		client := &http.Client{Timeout: 30 * time.Second}

		// Fetch all files from profile's selected projects
		var files []FileActivity
		for _, project := range profile.SelectedProjects {
			// Fetch files for this project
			url := fmt.Sprintf("https://api.figma.com/v1/projects/%s/files", project.ID)
			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				continue
			}
			req.Header.Set("X-Figma-Token", token)

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode != 200 {
				resp.Body.Close()
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var projectFilesResp struct {
				Files []struct {
					Key  string `json:"key"`
					Name string `json:"name"`
				} `json:"files"`
			}
			if err := json.Unmarshal(body, &projectFilesResp); err != nil {
				continue
			}

			// For each file, check if user modified it in time window
			for _, fileInfo := range projectFilesResp.Files {
				// Get file metadata
				fileURL := fmt.Sprintf("https://api.figma.com/v1/files/%s", fileInfo.Key)
				fileReq, err := http.NewRequest("GET", fileURL, nil)
				if err != nil {
					continue
				}
				fileReq.Header.Set("X-Figma-Token", token)

				fileResp, err := client.Do(fileReq)
				if err != nil {
					continue
				}

				if fileResp.StatusCode != 200 {
					fileResp.Body.Close()
					continue
				}

				fileBody, _ := io.ReadAll(fileResp.Body)
				fileResp.Body.Close()

				var fileData struct {
					Name         string    `json:"name"`
					LastModified time.Time `json:"lastModified"`
					Version      string    `json:"version"`
				}
				json.Unmarshal(fileBody, &fileData)

				// Get file version history to determine created date
				versionsURL := fmt.Sprintf("https://api.figma.com/v1/files/%s/versions", fileInfo.Key)
				versionsReq, err := http.NewRequest("GET", versionsURL, nil)
				if err == nil {
					versionsReq.Header.Set("X-Figma-Token", token)
					versionsResp, err := client.Do(versionsReq)
					if err == nil && versionsResp.StatusCode == 200 {
						versionsBody, _ := io.ReadAll(versionsResp.Body)
						versionsResp.Body.Close()

						var versionsData struct {
							Versions []struct {
								CreatedAt time.Time `json:"created_at"`
							} `json:"versions"`
						}
						json.Unmarshal(versionsBody, &versionsData)

						// Get earliest version (file creation date)
						var createdAt time.Time
						if len(versionsData.Versions) > 0 {
							createdAt = versionsData.Versions[len(versionsData.Versions)-1].CreatedAt
						}

						// Check if file was created in the time window
						createdInWindow := false
						if !createdAt.IsZero() && createdAt.After(window.Start) && createdAt.Before(window.End) {
							createdInWindow = true
						}

						// Check if file was modified in the time window
						myChanges := false
						if fileData.LastModified.After(window.Start) && fileData.LastModified.Before(window.End) {
							myChanges = true
						}

						// Only include files with activity (created or modified in window)
						if myChanges || createdInWindow {
							files = append(files, FileActivity{
								FileKey:         fileInfo.Key,
								FileName:        fileData.Name,
								ProjectName:     project.Name, // Use project name from profile
								LastModified:    fileData.LastModified,
								CreatedAt:       createdAt,
								MyChanges:       myChanges,
								CreatedInWindow: createdInWindow,
								Versions:        []FigmaVersion{},
								Comments:        []FigmaComment{},
							})
						}
					}
				}
			}
		}

		// Build report
		report := &ActivityReport{
			TimeWindow:   window,
			UserID:       userID,
			UserHandle:   userHandle,
			Files:        files,
			TotalFiles:   len(files),
			TotalChanges: 0,
			GeneratedAt:  time.Now(),
		}

		// Count total changes
		for _, file := range files {
			if file.MyChanges {
				report.TotalChanges++
			}
		}

		// Format report content
		content := formatReportMarkdown(report)

		return reportGeneratedMsg{
			report:  report,
			content: content,
		}
	}
}

func formatReportMarkdown(report *ActivityReport) string {
	var sb strings.Builder

	sb.WriteString("# Status Report\n")
	sb.WriteString(fmt.Sprintf("## From %s to %s\n",
		report.TimeWindow.Start.Format("2006-01-02"),
		report.TimeWindow.End.Format("2006-01-02")))

	// Add user information if available
	if report.UserHandle != "" {
		sb.WriteString(fmt.Sprintf("User: %s\n\n", report.UserHandle))
	} else {
		sb.WriteString("\n")
	}

	if len(report.Files) == 0 {
		sb.WriteString("No file activity found in the selected time period.\n")
	} else {
		// Group by project
		projectFiles := make(map[string][]FileActivity)
		for _, file := range report.Files {
			projectName := file.ProjectName
			if projectName == "" {
				projectName = "Unknown Project"
			}
			projectFiles[projectName] = append(projectFiles[projectName], file)
		}

		for projectName, files := range projectFiles {
			sb.WriteString(fmt.Sprintf("\n### %s\n\n", projectName))
			for _, file := range files {
				// Determine status
				status := "Modified"
				if file.CreatedInWindow {
					status = "Created"
				}

				// Create Figma file URL
				figmaURL := fmt.Sprintf("https://www.figma.com/file/%s", file.FileKey)

				// Format: - File name, link (Created/Modified)
				sb.WriteString(fmt.Sprintf("- [%s](%s) (%s)\n",
					file.FileName,
					figmaURL,
					status))
			}
		}
	}

	return sb.String()
}

func exportReport(content string, profileName string) tea.Cmd {
	return func() tea.Msg {
		// Create reports directory in current working directory
		reportsDir := "reports"
		if err := os.MkdirAll(reportsDir, 0755); err != nil {
			return reportExportErrMsg{err: "Failed to create reports directory: " + err.Error()}
		}

		// Generate filename with profile name and date
		if profileName == "" {
			profileName = "default"
		}
		timestamp := time.Now().Format("2006-01-02")
		filename := fmt.Sprintf("%s-%s.md", profileName, timestamp)
		filepath := filepath.Join(reportsDir, filename)

		// Write content to file
		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			return reportExportErrMsg{err: "Failed to write report: " + err.Error()}
		}

		return reportExportedMsg{filepath: filepath}
	}
}

func (m model) viewReportView() string {
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Build report display
	var contentStrings []string
	contentStrings = append(contentStrings, "")

	if m.generatingReport {
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render("  Generating Report..."))
		contentStrings = append(contentStrings, "")
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  Please wait while we fetch your Figma activity data..."))
	} else if m.reportError != "" {
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Bold(true).Render("  Error"))
		contentStrings = append(contentStrings, "")
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(defaultTextColor).Render("  "+m.reportError))
	} else if m.reportContent != "" {
		// Render markdown using glamour
		r, err := glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(m.width-4),
		)

		if err != nil {
			contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Render("  Failed to initialize markdown renderer"))
		} else {
			rendered, err := r.Render(m.reportContent)
			if err != nil {
				contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Render("  Failed to render markdown"))
			} else {
				// Add the rendered markdown
				contentStrings = append(contentStrings, rendered)
			}
		}

		// Show export success/error messages
		if m.exportSuccess != "" {
			contentStrings = append(contentStrings, "")
			contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#4fc06b")).Bold(true).Render("  ✓ Exported successfully!"))
			contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  "+m.exportSuccess))
		} else if m.exportError != "" {
			contentStrings = append(contentStrings, "")
			contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(lipgloss.Color("#ea4536")).Bold(true).Render("  ✗ Export failed"))
			contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(defaultTextColor).Render("  "+m.exportError))
		}
	}

	contentStrings = append(contentStrings, "")

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(contentStrings, "\n"))


	// Footer
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back to menu")
	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top, escStyle, " ", escDesc)

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
}

func (m model) viewReportConfig() string {
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

	// Header text
	titleText := "▨ FIGMA BEACON"
	statusText := m.profileStatus

	// Build configuration screen
	var contentStrings []string
	contentStrings = append(contentStrings, "")
	contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render("  Generate Activity Report"))
	contentStrings = append(contentStrings, "")

	// Display profile selection
	contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  Select profile:"))
	contentStrings = append(contentStrings, "")

	if len(m.profiles) == 0 {
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("    No profiles available"))
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("    Please create a profile first"))
	} else {
		// Show profiles horizontally with arrows
		var profileParts []string

		// Left arrow
		if m.reportProfileIndex > 0 {
			profileParts = append(profileParts, lipgloss.NewStyle().Foreground(cyanColor).Render(" ◀ "))
		} else {
			profileParts = append(profileParts, lipgloss.NewStyle().Foreground(dimWhiteColor).Render(" ◀ "))
		}

		// Current profile name
		selectedProfile := m.profiles[m.reportProfileIndex]
		profileName := selectedProfile.Name
		if selectedProfile.IsDefault {
			profileName += " (default)"
		}
		profileParts = append(profileParts, lipgloss.NewStyle().Foreground(whiteColor).Bold(true).Render(profileName))

		// Right arrow
		if m.reportProfileIndex < len(m.profiles)-1 {
			profileParts = append(profileParts, lipgloss.NewStyle().Foreground(cyanColor).Render(" ▶"))
		} else {
			profileParts = append(profileParts, lipgloss.NewStyle().Foreground(dimWhiteColor).Render(" ▶"))
		}

		profileLine := "   " + strings.Join(profileParts, "")
		contentStrings = append(contentStrings, profileLine)

		// Show profile counter
		counter := fmt.Sprintf("    %d / %d", m.reportProfileIndex+1, len(m.profiles))
		contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render(counter))
	}

	contentStrings = append(contentStrings, "")
	contentStrings = append(contentStrings, "")

	// Display time window options
	contentStrings = append(contentStrings, lipgloss.NewStyle().Foreground(dimWhiteColor).Render("  Select time window:"))
	contentStrings = append(contentStrings, "")

	for i, option := range m.reportTimeOptions {
		var optionColor lipgloss.Color
		var optionBold bool
		var prefix string

		if m.reportTimeIndex == i {
			optionColor = whiteColor
			optionBold = true
			prefix = "  → "
		} else {
			optionColor = defaultTextColor
			optionBold = false
			prefix = "    "
		}

		optionStyle := lipgloss.NewStyle().
			Foreground(optionColor).
			Bold(optionBold)

		contentStrings = append(contentStrings, optionStyle.Render(prefix+option))
	}

	contentStrings = append(contentStrings, "")
	contentStrings = append(contentStrings, "")

	content := lipgloss.NewStyle().
		Padding(0, 1).
		Background(bgColor).
		Render(strings.Join(contentStrings, "\n"))


	// Footer
	escStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("esc")
	escDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("back")
	enterStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("enter")
	enterDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("generate")
	arrowsStyle := lipgloss.NewStyle().Foreground(cyanColor).Render("←/→")
	arrowsDesc := lipgloss.NewStyle().Foreground(dimWhiteColor).Render("profile")

	leftShortcuts := lipgloss.JoinHorizontal(lipgloss.Top,
		escStyle, " ", escDesc, "    ",
		arrowsStyle, " ", arrowsDesc, "    ",
		enterStyle, " ", enterDesc)

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

	// Use responsive layout
	return createResponsiveLayout(m.width, m.height, bgColor, gradientColors, titleText, statusText, whiteColor, statusBgColor, content, footer)
}

// Helper to create responsive layout with footer at bottom
func createResponsiveLayout(width, height int, bgColor lipgloss.Color, gradientColors []lipgloss.Color, titleText, statusText string, whiteColor, statusBgColor lipgloss.Color, content, footer string) string {
	// Create header (3 lines)
	topGradientLine := createGradientBar(width, gradientColors)
	middleGradientLine := createGradientBarWithText(width, gradientColors, titleText, statusText, whiteColor, statusBgColor)
	bottomGradientLine := createGradientBar(width, gradientColors)

	// Create divider (1 line)
	divider := createGradientDivider(width, gradientColors)

	// Calculate heights
	headerHeight := 3
	dividerHeight := 1
	footerHeight := 1
	spacingHeight := 1 // Extra line below footer
	contentHeight := height - headerHeight - dividerHeight - footerHeight - spacingHeight

	if contentHeight < 1 {
		contentHeight = 1
	}

	// Make content fill available space
	contentRendered := lipgloss.NewStyle().
		Background(bgColor).
		Width(width).
		Height(contentHeight).
		Render(content)

	// Combine all sections vertically
	return lipgloss.JoinVertical(
		lipgloss.Left,
		topGradientLine,
		middleGradientLine,
		bottomGradientLine,
		contentRendered,
		divider,
		footer,
		"", // spacing line below footer
	)
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
