package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/zalando/go-keyring"

	"github.com/akashprasad/file-uploader/packages/sdk"
)

type screen int

const (
	providerScreen screen = iota
	setupScreen
	uploadFormScreen
	uploadingScreen
	resultScreen
)

type field struct {
	name     string
	input    textinput.Model
	required bool
}

type uploadFinishedMsg struct {
	url string
	err error
}

type model struct {
	screen    screen
	providers []sdk.Provider
	selected  int
	provider  sdk.Provider
	profile   map[string]string
	fields    []field
	focused   int
	spinner   spinner.Model
	resultURL string
	err       error
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

func initialModel() model {
	return model{
		providers: []sdk.Provider{
			sdk.ProviderFilesystem,
			sdk.ProviderGCS,
			sdk.ProviderSupabase,
			sdk.ProviderUpstash,
			sdk.ProviderVercel,
		},
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if key.String() == "ctrl+c" || key.String() == "q" && (m.screen == providerScreen || m.screen == resultScreen) {
			return m, tea.Quit
		}

		switch m.screen {
		case providerScreen:
			return m.updateProvider(key)
		case setupScreen:
			return m.updateSetup(key)
		case uploadFormScreen:
			return m.updateUploadForm(key)
		case resultScreen:
			switch key.String() {
			case "enter":
				m.screen = providerScreen
				m.err = nil
				m.resultURL = ""
			case "esc":
				m.screen = uploadFormScreen
			}
			return m, nil
		}
	}

	if m.screen == uploadingScreen {
		switch msg := msg.(type) {
		case uploadFinishedMsg:
			m.screen = resultScreen
			m.resultURL = msg.url
			m.err = msg.err
			return m, nil
		default:
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	if (m.screen == setupScreen || m.screen == uploadFormScreen) && len(m.fields) > 0 {
		var cmd tea.Cmd
		m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateProvider(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.providers)-1 {
			m.selected++
		}
	case "enter":
		m.provider = m.providers[m.selected]
		m.err = nil
		if m.provider == sdk.ProviderFilesystem {
			m.profile = map[string]string{"Root directory": "./uploads"}
			return m.enterUploadForm()
		}
		profile, err := loadProfile(m.provider)
		if err == nil {
			m.profile = profile
			return m.enterUploadForm()
		}
		if !errors.Is(err, keyring.ErrNotFound) {
			m.screen = providerScreen
			m.err = err
			return m, nil
		}
		m.profile = nil
		m.fields = setupFieldsFor(m.provider, nil)
		m.focused = 0
		m.screen = setupScreen
		return m, m.fields[0].input.Focus()
	}
	return m, nil
}

func (m model) updateSetup(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.screen = providerScreen
		return m, nil
	case "tab", "down":
		return m.moveFocus(1)
	case "shift+tab", "up":
		return m.moveFocus(-1)
	case "enter":
		if m.focused < len(m.fields)-1 {
			return m.moveFocus(1)
		}
		values, err := m.values()
		if err != nil {
			m.err = err
			return m, nil
		}
		if m.provider != sdk.ProviderFilesystem {
			if err := saveProfile(m.provider, values); err != nil {
				m.err = fmt.Errorf("save credentials to system keychain: %w", err)
				return m, nil
			}
		}
		m.profile = values
		m.err = nil
		return m.enterUploadForm()
	}

	var cmd tea.Cmd
	m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(key)
	return m, cmd
}

func (m model) updateUploadForm(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.String() == "ctrl+e" && m.provider != sdk.ProviderFilesystem {
		m.fields = setupFieldsFor(m.provider, m.profile)
		m.focused = 0
		m.screen = setupScreen
		return m, m.fields[0].input.Focus()
	}
	switch key.String() {
	case "esc":
		m.screen = providerScreen
		return m, nil
	case "tab", "down":
		return m.moveFocus(1)
	case "shift+tab", "up":
		return m.moveFocus(-1)
	case "enter":
		if m.focused < len(m.fields)-1 {
			return m.moveFocus(1)
		}
		values, err := m.values()
		if err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		m.screen = uploadingScreen
		return m, tea.Batch(m.spinner.Tick, upload(m.provider, m.profile, values))
	}
	var cmd tea.Cmd
	m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(key)
	return m, cmd
}

func (m model) enterUploadForm() (tea.Model, tea.Cmd) {
	m.fields = uploadFieldsFor(m.provider)
	m.focused = 0
	m.screen = uploadFormScreen
	return m, m.fields[0].input.Focus()
}

func (m model) moveFocus(change int) (tea.Model, tea.Cmd) {
	m.fields[m.focused].input.Blur()
	m.focused = (m.focused + change + len(m.fields)) % len(m.fields)
	return m, m.fields[m.focused].input.Focus()
}

func (m model) values() (map[string]string, error) {
	values := make(map[string]string, len(m.fields))
	for _, field := range m.fields {
		value := strings.TrimSpace(field.input.Value())
		if field.required && value == "" {
			return nil, fmt.Errorf("%s is required", field.name)
		}
		values[field.name] = value
	}
	return values, nil
}

func setupFieldsFor(provider sdk.Provider, values map[string]string) []field {
	get := func(name string) string { return values[name] }
	if provider == sdk.ProviderFilesystem {
		return []field{newField("Root directory", "./uploads", get("Root directory"), false, true)}
	}
	switch provider {
	case sdk.ProviderGCS:
		return []field{
			newField("OAuth access token", "Google Cloud OAuth token", get("OAuth access token"), true, true),
		}
	case sdk.ProviderUpstash:
		return []field{
			newField("S3 endpoint", "https://...", get("S3 endpoint"), false, true),
			newField("Region", "Upstash region", get("Region"), false, true),
			newField("Bucket", "Upstash bucket", get("Bucket"), false, true),
			newField("S3 access key", "S3 access key", get("S3 access key"), true, true),
			newField("S3 secret key", "S3 secret key", get("S3 secret key"), true, true),
			newField("S3 session token (optional)", "", get("S3 session token (optional)"), true, false),
			newField("Public base URL (optional)", "", get("Public base URL (optional)"), false, false),
		}
	case sdk.ProviderVercel:
		return []field{
			newField("Blob token", "Vercel Blob token", get("Blob token"), true, true),
			newField("Store ID", "Vercel Blob store ID", get("Store ID"), false, true),
			newField("Access (public/private)", "public", defaultValue(get("Access (public/private)"), "public"), false, true),
		}
	default:
		return []field{
			newField("Project URL", "https://project.supabase.co", defaultValue(get("Project URL"), os.Getenv("SUPABASE_PROJECT_URL")), false, true),
			newField("API key", "Supabase API key", defaultValue(get("API key"), os.Getenv("SUPABASE_API_KEY")), true, true),
			newField("Access token (optional)", "User JWT", get("Access token (optional)"), true, false),
		}
	}
}

func uploadFieldsFor(provider sdk.Provider) []field {
	source := newField("Source file", "./path/to/file", "", false, true)
	key := newField("Object key", "images/photo.png", "", false, true)
	if provider == sdk.ProviderUpstash || provider == sdk.ProviderVercel {
		return []field{source, key}
	}
	bucketDefault := "assets"
	if provider == sdk.ProviderFilesystem {
		bucketDefault = "local"
	}
	return []field{source, newField("Bucket", bucketDefault, "", false, true), key}
}

func defaultValue(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func newField(name, placeholder, value string, secret, required bool) field {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = placeholder
	input.SetWidth(52)
	input.SetValue(value)
	if secret {
		input.EchoMode = textinput.EchoPassword
	}
	return field{name: name, input: input, required: required}
}

func upload(provider sdk.Provider, profile, values map[string]string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(values["Source file"])
		if err != nil {
			return uploadFinishedMsg{err: fmt.Errorf("open source file: %w", err)}
		}
		defer file.Close()

		var config sdk.Config
		switch provider {
		case sdk.ProviderFilesystem:
			filesystemConfig := sdk.FilesystemConfig{RootDir: profile["Root directory"]}
			config = sdk.Config{Provider: provider, Filesystem: &filesystemConfig}
		case sdk.ProviderGCS:
			token := profile["OAuth access token"]
			gcsConfig := sdk.GCSConfig{TokenSource: func(context.Context) (string, error) { return token, nil }}
			config = sdk.Config{Provider: provider, GCS: &gcsConfig}
		case sdk.ProviderSupabase:
			supabaseConfig := sdk.SupabaseConfig{ProjectURL: profile["Project URL"], APIKey: profile["API key"]}
			if token := profile["Access token (optional)"]; token != "" {
				supabaseConfig.TokenSource = func(context.Context) (string, error) { return token, nil }
			}
			config = sdk.Config{Provider: provider, Supabase: &supabaseConfig}
		case sdk.ProviderUpstash:
			credentials := sdk.UpstashCredentials{
				AccessKey: profile["S3 access key"], SecretKey: profile["S3 secret key"], SessionToken: profile["S3 session token (optional)"],
			}
			upstashConfig := sdk.UpstashConfig{
				Endpoint: profile["S3 endpoint"], Region: profile["Region"], Bucket: profile["Bucket"],
				PublicBaseURL:     profile["Public base URL (optional)"],
				CredentialsSource: func(context.Context) (sdk.UpstashCredentials, error) { return credentials, nil },
			}
			config = sdk.Config{Provider: provider, Upstash: &upstashConfig}
		case sdk.ProviderVercel:
			vercelConfig := sdk.VercelConfig{Token: profile["Blob token"], StoreID: profile["Store ID"], Access: profile["Access (public/private)"]}
			config = sdk.Config{Provider: provider, Vercel: &vercelConfig}
		default:
			return uploadFinishedMsg{err: fmt.Errorf("unsupported TUI provider %q", provider)}
		}

		client, err := sdk.New(config)
		if err != nil {
			return uploadFinishedMsg{err: err}
		}
		result, err := client.Upload(context.Background(), sdk.UploadRequest{
			Source: file,
			Bucket: values["Bucket"],
			Key:    values["Object key"],
		})
		return uploadFinishedMsg{url: result.URL, err: err}
	}
}

func (m model) View() tea.View {
	var content strings.Builder
	content.WriteString(titleStyle.Render("File Uploader"))
	content.WriteString("\n\n")

	switch m.screen {
	case providerScreen:
		content.WriteString("Choose a provider\n\n")
		for index, provider := range m.providers {
			prefix := "  "
			name := providerName(provider)
			if index == m.selected {
				prefix = "› "
				name = selectedStyle.Render(name)
			}
			fmt.Fprintf(&content, "%s%s\n", prefix, name)
		}
		content.WriteString("\n")
		if m.err != nil {
			content.WriteString(errorStyle.Render(m.err.Error()))
			content.WriteString("\n\n")
		}
		content.WriteString(mutedStyle.Render("↑/↓ select • enter continue • q quit"))
	case setupScreen:
		fmt.Fprintf(&content, "Set up %s\n\n", selectedStyle.Render(providerName(m.provider)))
		content.WriteString(mutedStyle.Render("Credentials are saved in your system keychain.\n"))
		content.WriteString(mutedStyle.Render("\n"))
		for index, field := range m.fields {
			label := field.name
			if index == m.focused {
				label = selectedStyle.Render(label)
			}
			fmt.Fprintf(&content, "%s\n%s\n\n", label, field.input.View())
		}
		if m.err != nil {
			content.WriteString(errorStyle.Render(m.err.Error()))
			content.WriteString("\n\n")
		}
		content.WriteString(mutedStyle.Render("tab next • shift+tab previous • enter save and continue • esc back"))
	case uploadFormScreen:
		fmt.Fprintf(&content, "Provider: %s\n\n", selectedStyle.Render(providerName(m.provider)))
		for index, field := range m.fields {
			label := field.name
			if index == m.focused {
				label = selectedStyle.Render(label)
			}
			fmt.Fprintf(&content, "%s\n%s\n\n", label, field.input.View())
		}
		if m.err != nil {
			content.WriteString(errorStyle.Render(m.err.Error()))
			content.WriteString("\n\n")
		}
		if m.provider == sdk.ProviderFilesystem {
			content.WriteString(mutedStyle.Render("tab next • shift+tab previous • enter upload • esc providers"))
		} else {
			content.WriteString(mutedStyle.Render("tab next • shift+tab previous • enter upload • ctrl+e credentials • esc providers"))
		}
	case uploadingScreen:
		fmt.Fprintf(&content, "%s Uploading to %s…\n", m.spinner.View(), m.provider)
	case resultScreen:
		if m.err != nil {
			content.WriteString(errorStyle.Render("Upload failed"))
			content.WriteString("\n\n")
			content.WriteString(m.err.Error())
			content.WriteString("\n\n")
			content.WriteString(mutedStyle.Render("esc edit • enter start over • q quit"))
		} else {
			content.WriteString(successStyle.Render("Upload complete"))
			content.WriteString("\n\n")
			content.WriteString(m.resultURL)
			content.WriteString("\n\n")
			content.WriteString(mutedStyle.Render("enter upload another • q quit"))
		}
	}

	view := tea.NewView(lipgloss.NewStyle().Padding(1, 2).Render(content.String()))
	view.AltScreen = true
	view.WindowTitle = "File Uploader"
	return view
}

func providerName(provider sdk.Provider) string {
	switch provider {
	case sdk.ProviderFilesystem:
		return "Local filesystem"
	case sdk.ProviderGCS:
		return "Google Cloud Storage"
	case sdk.ProviderSupabase:
		return "Supabase Storage"
	case sdk.ProviderUpstash:
		return "Upstash Blob"
	case sdk.ProviderVercel:
		return "Vercel Blob"
	default:
		return string(provider)
	}
}

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
