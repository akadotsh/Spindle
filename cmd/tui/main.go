package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	actionScreen
	uploadFormScreen
	uploadingScreen
	resultScreen
	bucketLoadingScreen
	bucketScreen
	listingScreen
	browserScreen
	objectConfirmScreen
	downloadingScreen
	deletingScreen
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

type objectsListedMsg struct {
	page sdk.ListResult
	err  error
}
type bucketsListedMsg struct {
	buckets []string
	err     error
}
type objectFinishedMsg struct {
	message string
	err     error
}

type model struct {
	screen     screen
	providers  []sdk.Provider
	selected   int
	provider   sdk.Provider
	profile    map[string]string
	fields     []field
	focused    int
	spinner    spinner.Model
	resultURL  string
	err        error
	action     int
	objects    []sdk.Object
	buckets    []string
	bucketSel  int
	objectSel  int
	bucket     string
	prefix     string
	cursor     string
	nextCursor string
	status     string
	deleteKey  string
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
		case actionScreen:
			switch key.String() {
			case "up", "k", "down", "j":
				m.action = 1 - m.action
			case "enter":
				if m.action == 0 {
					return m.enterUploadForm()
				}
				return m.enterBrowseForm()
			case "esc":
				m.screen = providerScreen
			}
			return m, nil
		case bucketScreen:
			switch key.String() {
			case "up", "k":
				if m.bucketSel > 0 {
					m.bucketSel--
				}
			case "down", "j":
				if m.bucketSel < len(m.buckets)-1 {
					m.bucketSel++
				}
			case "enter":
				if len(m.buckets) > 0 {
					m.bucket = m.buckets[m.bucketSel]
					m.prefix = ""
					m.status = ""
					m.screen = listingScreen
					return m, tea.Batch(m.spinner.Tick, listObjects(m.provider, m.profile, m.bucket, m.prefix, ""))
				}
			case "esc":
				return m.enterActionScreen()
			}
			return m, nil
		case browserScreen:
			return m.updateBrowser(key)
		case objectConfirmScreen:
			switch key.String() {
			case "y":
				m.screen = deletingScreen
				return m, tea.Batch(m.spinner.Tick, deleteObject(m.provider, m.profile, m.bucket, m.deleteKey))
			case "n", "esc":
				m.screen = browserScreen
				m.deleteKey = ""
			}
			return m, nil
		case resultScreen:
			switch key.String() {
			case "enter":
				m.screen = actionScreen
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
	if m.screen == bucketLoadingScreen {
		switch msg := msg.(type) {
		case bucketsListedMsg:
			m.buckets = msg.buckets
			m.bucketSel = 0
			m.err = msg.err
			m.screen = bucketScreen
			return m, nil
		default:
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	if m.screen == listingScreen || m.screen == deletingScreen || m.screen == downloadingScreen {
		switch msg := msg.(type) {
		case objectsListedMsg:
			m.screen = browserScreen
			m.err = msg.err
			m.objects = msg.page.Objects
			m.nextCursor = msg.page.NextCursor
			m.cursor = ""
			m.objectSel = 0
			return m, nil
		case objectFinishedMsg:
			m.screen = browserScreen
			m.err = msg.err
			m.status = msg.message
			if msg.err == nil && msg.message == "Object deleted" {
				m.deleteKey = ""
				m.screen = listingScreen
				return m, tea.Batch(m.spinner.Tick, listObjects(m.provider, m.profile, m.bucket, m.prefix, ""))
			}
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
			return m.enterActionScreen()
		}
		profile, err := loadProfile(m.provider)
		if err == nil {
			m.profile = profile
			return m.enterActionScreen()
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
		return m.enterActionScreen()
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

func (m model) enterActionScreen() (tea.Model, tea.Cmd) {
	m.screen = actionScreen
	m.action = 0
	m.err = nil
	return m, nil
}

func (m model) enterBrowseForm() (tea.Model, tea.Cmd) {
	m.screen = bucketLoadingScreen
	m.err = nil
	return m, tea.Batch(m.spinner.Tick, listBuckets(m.provider, m.profile))
}

func (m model) updateBrowser(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		return m.enterActionScreen()
	case "up", "k":
		if m.objectSel > 0 {
			m.objectSel--
		}
	case "down", "j":
		if m.objectSel < len(m.objects)-1 {
			m.objectSel++
		}
	case "n":
		if m.nextCursor != "" {
			m.cursor = m.nextCursor
			m.screen = listingScreen
			return m, tea.Batch(m.spinner.Tick, listObjects(m.provider, m.profile, m.bucket, m.prefix, m.cursor))
		}
	case "r":
		m.screen = listingScreen
		return m, tea.Batch(m.spinner.Tick, listObjects(m.provider, m.profile, m.bucket, m.prefix, ""))
	case "d":
		if len(m.objects) > 0 {
			m.screen = downloadingScreen
			return m, tea.Batch(m.spinner.Tick, downloadObject(m.provider, m.profile, m.objects[m.objectSel]))
		}
	case "x":
		if len(m.objects) > 0 {
			m.deleteKey = m.objects[m.objectSel].Key
			m.screen = objectConfirmScreen
		}
	}
	return m, nil
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
			newField("Google Cloud project ID", "my-project-id", get("Google Cloud project ID"), false, true),
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

		client, err := clientFor(provider, profile)
		if err != nil {
			return uploadFinishedMsg{err: err}
		}
		result, err := client.Upload(context.Background(), sdk.UploadRequest{Source: file, Bucket: values["Bucket"], Key: values["Object key"]})
		return uploadFinishedMsg{url: result.URL, err: err}
	}
}

func clientFor(provider sdk.Provider, profile map[string]string) (*sdk.Client, error) {
	var config sdk.Config
	switch provider {
	case sdk.ProviderFilesystem:
		filesystemConfig := sdk.FilesystemConfig{RootDir: profile["Root directory"]}
		config = sdk.Config{Provider: provider, Filesystem: &filesystemConfig}
	case sdk.ProviderGCS:
		token := profile["OAuth access token"]
		gcsConfig := sdk.GCSConfig{ProjectID: profile["Google Cloud project ID"], TokenSource: func(context.Context) (string, error) { return token, nil }}
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
		return nil, fmt.Errorf("unsupported TUI provider %q", provider)
	}
	return sdk.New(config)
}

func listObjects(provider sdk.Provider, profile map[string]string, bucket, prefix, cursor string) tea.Cmd {
	return func() tea.Msg {
		client, err := clientFor(provider, profile)
		if err != nil {
			return objectsListedMsg{err: err}
		}
		page, err := client.List(context.Background(), sdk.ListRequest{Bucket: bucket, Prefix: prefix, Cursor: cursor, Limit: 100})
		return objectsListedMsg{page: page, err: err}
	}
}

func listBuckets(provider sdk.Provider, profile map[string]string) tea.Cmd {
	return func() tea.Msg {
		client, err := clientFor(provider, profile)
		if err != nil {
			return bucketsListedMsg{err: err}
		}
		buckets, err := client.ListBuckets(context.Background())
		return bucketsListedMsg{buckets: buckets, err: err}
	}
}

func downloadObject(provider sdk.Provider, profile map[string]string, object sdk.Object) tea.Cmd {
	return func() tea.Msg {
		client, err := clientFor(provider, profile)
		if err != nil {
			return objectFinishedMsg{err: err}
		}
		result, err := client.Get(context.Background(), sdk.GetRequest{Bucket: object.Bucket, Key: object.Key})
		if err != nil {
			return objectFinishedMsg{err: err}
		}
		defer result.Body.Close()
		name := filepath.Base(object.Key)
		if name == "." || name == string(filepath.Separator) || name == "" {
			return objectFinishedMsg{err: fmt.Errorf("invalid object key")}
		}
		out, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return objectFinishedMsg{err: fmt.Errorf("create download %s: %w", name, err)}
		}
		_, copyErr := io.Copy(out, result.Body)
		closeErr := out.Close()
		if copyErr != nil {
			_ = os.Remove(name)
			return objectFinishedMsg{err: fmt.Errorf("write download: %w", copyErr)}
		}
		if closeErr != nil {
			_ = os.Remove(name)
			return objectFinishedMsg{err: closeErr}
		}
		return objectFinishedMsg{message: "Downloaded to " + name}
	}
}

func deleteObject(provider sdk.Provider, profile map[string]string, bucket, key string) tea.Cmd {
	return func() tea.Msg {
		client, err := clientFor(provider, profile)
		if err != nil {
			return objectFinishedMsg{err: err}
		}
		err = client.Delete(context.Background(), sdk.DeleteRequest{Bucket: bucket, Key: key})
		message := "Object deleted"
		if err != nil {
			message = ""
		}
		return objectFinishedMsg{message: message, err: err}
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
	case actionScreen:
		fmt.Fprintf(&content, "Provider: %s\n\nChoose an action\n\n", selectedStyle.Render(providerName(m.provider)))
		for i, label := range []string{"Upload a file", "Browse objects"} {
			mark := "  "
			if i == m.action {
				mark = "› "
				label = selectedStyle.Render(label)
			}
			fmt.Fprintf(&content, "%s%s\n", mark, label)
		}
		content.WriteString("\n" + mutedStyle.Render("↑/↓ select • enter continue • esc providers"))
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
	case bucketLoadingScreen:
		fmt.Fprintf(&content, "%s Fetching buckets for %s…\n", m.spinner.View(), providerName(m.provider))
	case bucketScreen:
		fmt.Fprintf(&content, "Choose a bucket or store (%d)\n\n", len(m.buckets))
		content.WriteString(mutedStyle.Render(bucketScope(m.provider)) + "\n\n")
		for i, bucket := range m.buckets {
			mark := "  "
			if i == m.bucketSel {
				mark = "› "
				bucket = selectedStyle.Render(bucket)
			}
			fmt.Fprintf(&content, "%s%s\n", mark, bucket)
		}
		if len(m.buckets) == 0 && m.err == nil {
			content.WriteString(mutedStyle.Render("No buckets found.\n"))
		}
		if m.err != nil {
			content.WriteString(errorStyle.Render(m.err.Error()) + "\n")
		}
		content.WriteString("\n" + mutedStyle.Render("↑/↓ select • enter open bucket • esc actions"))
	case uploadingScreen:
		fmt.Fprintf(&content, "%s Uploading to %s…\n", m.spinner.View(), m.provider)
	case listingScreen:
		fmt.Fprintf(&content, "%s Loading objects from %s…\n", m.spinner.View(), m.provider)
	case downloadingScreen:
		fmt.Fprintf(&content, "%s Downloading object…\n", m.spinner.View())
	case deletingScreen:
		fmt.Fprintf(&content, "%s Deleting object…\n", m.spinner.View())
	case objectConfirmScreen:
		fmt.Fprintf(&content, "Delete object?\n\n%s\n\n", m.deleteKey)
		content.WriteString(mutedStyle.Render("y confirm • n/esc cancel"))
	case browserScreen:
		fmt.Fprintf(&content, "Objects in %s", m.bucket)
		if m.prefix != "" {
			fmt.Fprintf(&content, " (%s)", m.prefix)
		}
		content.WriteString("\n\n")
		if len(m.objects) == 0 {
			content.WriteString(mutedStyle.Render("No objects found.\n"))
		} else {
			for i, obj := range m.objects {
				mark := "  "
				key := obj.Key
				if i == m.objectSel {
					mark = "› "
					key = selectedStyle.Render(key)
				}
				fmt.Fprintf(&content, "%s%s  %s\n", mark, key, formatSize(obj.Size))
			}
		}
		if m.status != "" {
			content.WriteString("\n" + successStyle.Render(m.status))
		}
		if m.err != nil {
			content.WriteString("\n" + errorStyle.Render(m.err.Error()))
		}
		content.WriteString("\n\n")
		help := "↑/↓ select • d download • x delete • r refresh • esc actions"
		if m.nextCursor != "" {
			help = "↑/↓ select • n next page • d download • x delete • r refresh • esc actions"
		}
		content.WriteString(mutedStyle.Render(help))
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

func bucketScope(provider sdk.Provider) string {
	switch provider {
	case sdk.ProviderUpstash:
		return "Showing the bucket configured for this scoped token."
	case sdk.ProviderVercel:
		return "Showing the configured Vercel Blob store."
	default:
		return "Showing buckets visible to these credentials."
	}
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f KB", float64(size)/1024)
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
