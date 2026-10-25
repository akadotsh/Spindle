package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/akashprasad/file-uploader/packages/sdk"
)

type screen int

const (
	providerScreen screen = iota
	formScreen
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
		if key.String() == "ctrl+c" || key.String() == "q" && m.screen != formScreen {
			return m, tea.Quit
		}

		switch m.screen {
		case providerScreen:
			return m.updateProvider(key)
		case formScreen:
			return m.updateForm(key)
		case resultScreen:
			switch key.String() {
			case "enter":
				m.screen = providerScreen
				m.err = nil
				m.resultURL = ""
			case "esc":
				m.screen = formScreen
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

	if m.screen == formScreen && len(m.fields) > 0 {
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
		m.fields = fieldsFor(m.provider)
		m.focused = 0
		m.screen = formScreen
		return m, m.fields[0].input.Focus()
	}
	return m, nil
}

func (m model) updateForm(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		return m, tea.Batch(m.spinner.Tick, upload(m.provider, values))
	}

	var cmd tea.Cmd
	m.fields[m.focused].input, cmd = m.fields[m.focused].input.Update(key)
	return m, cmd
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

func fieldsFor(provider sdk.Provider) []field {
	source := newField("Source file", "./path/to/file", "", false, true)
	bucket := newField("Bucket", "assets", "", false, true)
	key := newField("Object key", "images/photo.png", "", false, true)
	switch provider {
	case sdk.ProviderFilesystem:
		return []field{
			newField("Root directory", "./uploads", "./uploads", false, true),
			source,
			newField("Bucket", "local", "", false, true),
			key,
		}
	case sdk.ProviderGCS:
		return []field{
			newField("OAuth access token", "Google Cloud OAuth token", "", true, true),
			source, bucket, key,
		}
	case sdk.ProviderUpstash:
		return []field{
			newField("S3 endpoint", "https://...", "", false, true),
			newField("Region", "Upstash region", "", false, true),
			newField("Bucket", "Upstash bucket", "", false, true),
			newField("S3 access key", "S3 access key", "", true, true),
			newField("S3 secret key", "S3 secret key", "", true, true),
			newField("S3 session token (optional)", "", "", true, false),
			newField("Public base URL (optional)", "", "", false, false),
			source, key,
		}
	case sdk.ProviderVercel:
		return []field{
			newField("Blob token", "Vercel Blob token", "", true, true),
			newField("Store ID", "Vercel Blob store ID", "", false, true),
			newField("Access (public/private)", "public", "public", false, true),
			source, key,
		}
	default:
		return []field{
			newField("Project URL", "https://project.supabase.co", os.Getenv("SUPABASE_PROJECT_URL"), false, true),
			newField("API key", "Supabase API key", os.Getenv("SUPABASE_API_KEY"), true, true),
			newField("Access token (optional)", "User JWT", "", true, false),
			source, bucket, key,
		}
	}
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

func upload(provider sdk.Provider, values map[string]string) tea.Cmd {
	return func() tea.Msg {
		file, err := os.Open(values["Source file"])
		if err != nil {
			return uploadFinishedMsg{err: fmt.Errorf("open source file: %w", err)}
		}
		defer file.Close()

		var config sdk.Config
		switch provider {
		case sdk.ProviderFilesystem:
			filesystemConfig := sdk.FilesystemConfig{RootDir: values["Root directory"]}
			config = sdk.Config{Provider: provider, Filesystem: &filesystemConfig}
		case sdk.ProviderGCS:
			token := values["OAuth access token"]
			gcsConfig := sdk.GCSConfig{TokenSource: func(context.Context) (string, error) { return token, nil }}
			config = sdk.Config{Provider: provider, GCS: &gcsConfig}
		case sdk.ProviderSupabase:
			supabaseConfig := sdk.SupabaseConfig{ProjectURL: values["Project URL"], APIKey: values["API key"]}
			if token := values["Access token (optional)"]; token != "" {
				supabaseConfig.TokenSource = func(context.Context) (string, error) { return token, nil }
			}
			config = sdk.Config{Provider: provider, Supabase: &supabaseConfig}
		case sdk.ProviderUpstash:
			credentials := sdk.UpstashCredentials{
				AccessKey: values["S3 access key"], SecretKey: values["S3 secret key"], SessionToken: values["S3 session token (optional)"],
			}
			upstashConfig := sdk.UpstashConfig{
				Endpoint: values["S3 endpoint"], Region: values["Region"], Bucket: values["Bucket"],
				PublicBaseURL:     values["Public base URL (optional)"],
				CredentialsSource: func(context.Context) (sdk.UpstashCredentials, error) { return credentials, nil },
			}
			config = sdk.Config{Provider: provider, Upstash: &upstashConfig}
		case sdk.ProviderVercel:
			vercelConfig := sdk.VercelConfig{Token: values["Blob token"], StoreID: values["Store ID"], Access: values["Access (public/private)"]}
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
			name := string(provider)
			if index == m.selected {
				prefix = "› "
				name = selectedStyle.Render(name)
			}
			fmt.Fprintf(&content, "%s%s\n", prefix, name)
		}
		content.WriteString("\n")
		content.WriteString(mutedStyle.Render("↑/↓ select • enter continue • q quit"))
	case formScreen:
		fmt.Fprintf(&content, "Provider: %s\n\n", selectedStyle.Render(string(m.provider)))
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
		content.WriteString(mutedStyle.Render("tab next • shift+tab previous • enter upload • esc back"))
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

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
