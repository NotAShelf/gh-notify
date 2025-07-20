package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"encoding/json"
	"path/filepath"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	// Github's REST API is now versioned. This is, as far as I can tell, the latest version
	// as of July 2025. Once GitHub updates their API version, this constant should be updated.
	// https://docs.github.com/en/rest/about-the-rest-api/api-versions?apiVersion=2022-11-28
	ghRestApiVersion     = "2022-11-28"
	ghNotifyPerPageLimit = 50
	finalMsg             = "All caught up!"
	minFzfVersion        = "0.29.0"
)

var (
	cacheEnabled  bool
	cacheDuration time.Duration
	cacheDir      string
	cacheMutex    sync.Mutex
)

var (
	green     = color.New(color.FgGreen).SprintFunc()
	darkGray  = color.New(color.FgHiBlack).SprintFunc()
	whiteBold = color.New(color.FgWhite, color.Bold).SprintFunc()
)

var (
	ghNotifyViewKey,
	ghNotifyTogglePreviewKey,
	ghNotifyToggleHelpKey string
	ghNotifyDebugMode bool
	ghNotifyVerbose   bool
)

type Notification struct {
	ID         string `json:"id"`
	Unread     bool   `json:"unread"`
	UpdatedAt  string `json:"updated_at"`
	LastReadAt string `json:"last_read_at"`
	Repository struct {
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
	Reason  string `json:"reason"`
	Subject struct {
		Type             string `json:"type"`
		Title            string `json:"title"`
		URL              string `json:"url"`
		LatestCommentURL string `json:"latest_comment_url"`
	} `json:"subject"`
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "ERROR:", msg)
	os.Exit(1)
}

func printHelpText(cmd *cobra.Command) {
	fmt.Printf("%sUsage%s\n  %s\n\n", whiteBold(""), "", cmd.UseLine())
	fmt.Printf("%sFlags%s\n", whiteBold(""), "")
	maxlen := 0
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		l := len(f.Name)
		if f.Shorthand != "" {
			l += 4 // " -x,"
		}
		if l > maxlen {
			maxlen = l
		}
	})
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		var flagStr string
		if f.Shorthand != "" {
			flagStr = fmt.Sprintf("  %s, %s", green("-"+f.Shorthand), green("--"+f.Name))
		} else {
			flagStr = fmt.Sprintf("      %s", green("--"+f.Name))
		}
		padding := maxlen + 6 - len(strings.ReplaceAll(flagStr, "\x1b[0m", ""))
		if padding < 2 {
			padding = 2
		}
		desc := f.Usage
		if f.DefValue != "" && f.DefValue != "false" {
			desc += fmt.Sprintf(" (default: %s)", f.DefValue)
		}
		fmt.Printf("%s%s%s\n", flagStr, strings.Repeat(" ", padding), desc)
	})
	fmt.Printf("\n%sKey Bindings fzf%s\n", whiteBold(""), "")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyToggleHelpKey), "toggle help")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyViewKey), "view the selected notification in the 'less' pager")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyTogglePreviewKey), "toggle notification preview")
	fmt.Printf("  %-10s  %s\n\n", green("esc"), "quit")
}

func ghRestApiClient() *api.RESTClient {
	client, err := api.NewRESTClient(api.ClientOptions{
		Headers: map[string]string{
			"X-GitHub-Api-Version": ghRestApiVersion,
		},
	})
	if err != nil {
		die(fmt.Sprintf("failed to create REST client: %v", err))
	}
	return client
}

func cachePath(key string) string {
	return filepath.Join(cacheDir, key+".json")
}

func cacheGet(key string, v any) bool {
	if !cacheEnabled {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] disabled, skipping cache for key: %s\n", key)
		}
		return false
	}
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	path := cachePath(key)
	f, err := os.Open(path)
	if err != nil {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] miss (file not found) for key: %s\n", key)
		}
		return false
	}
	defer f.Close()
	var entry struct {
		Timestamp int64           `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(f).Decode(&entry); err != nil {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] miss (decode error) for key: %s: %v\n", key, err)
		}
		return false
	}
	if time.Since(time.Unix(entry.Timestamp, 0)) > cacheDuration {
		_ = os.Remove(path)
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] expired for key: %s\n", key)
		}
		return false
	}
	if ghNotifyVerbose {
		fmt.Fprintf(os.Stderr, "[cache] hit for key: %s\n", key)
	}
	return json.Unmarshal(entry.Data, v) == nil
}

func cacheSet(key string, v any) {
	if !cacheEnabled {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] disabled, not storing key: %s\n", key)
		}
		return
	}
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	path := cachePath(key)
	data, err := json.Marshal(v)
	if err != nil {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] marshal error for key: %s: %v\n", key, err)
		}
		return
	}
	entry := struct {
		Timestamp int64           `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}{
		Timestamp: time.Now().Unix(),
		Data:      data,
	}
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] file create error for key: %s: %v\n", key, err)
		}
		return
	}
	if err := json.NewEncoder(f).Encode(entry); err != nil {
		f.Close()
		os.Remove(tmpPath)
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[cache] encode error for key: %s: %v\n", key, err)
		}
		return
	}
	f.Close()
	os.Rename(tmpPath, path)
	if ghNotifyVerbose {
		fmt.Fprintf(os.Stderr, "[cache] stored key: %s\n", key)
	}
}

func getNotifs(pageNum int, onlyParticipating, includeAll bool) ([]Notification, error) {
	var notifs []Notification
	endpoint := fmt.Sprintf("notifications?per_page=%d&page=%d&participating=%t&all=%t",
		ghNotifyPerPageLimit, pageNum, onlyParticipating, includeAll)
	cacheKey := fmt.Sprintf("notifs_%d_%t_%t", pageNum, onlyParticipating, includeAll)
	if cacheGet(cacheKey, &notifs) {
		if ghNotifyVerbose {
			fmt.Fprintf(os.Stderr, "[api] notifications page %d served from cache\n", pageNum)
		}
		return notifs, nil
	}
	client := ghRestApiClient()
	if ghNotifyVerbose {
		fmt.Fprintf(os.Stderr, "[api] fetching notifications page %d from GitHub\n", pageNum)
	}
	if err := client.Get(endpoint, &notifs); err != nil {
		return nil, err
	}
	cacheSet(cacheKey, notifs)
	return notifs, nil
}

func shortDate(dt string) string {
	t, err := time.Parse(time.RFC3339, dt)
	if err != nil {
		return "2020"
	}
	return t.Format("2006-01")
}

func isoTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func lastPathComponent(url string) string {
	if url == "" {
		return ""
	}
	parts := strings.Split(url, "/")
	return parts[len(parts)-1]
}

func abbreviate(s string, max int) string {
	if max <= 1 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func markAllRead(isoTime string) error {
	client := ghRestApiClient()
	body := map[string]any{
		"last_read_at": isoTime,
		"read":         true,
	}
	return client.Put("notifications", nil, body)
}

func main() {
	initConfig()

	var (
		exclusion, filter, updateSubscriptionURL string
		numNotifications                         int
		onlyParticipating, includeAll            bool
		printStatic, markRead                    bool
	)

	rootCmd := &cobra.Command{
		Use:   "gh-notify",
		Short: "GitHub notifications CLI",
		Long:  "GitHub CLI extension to display GitHub notifications",
		Run: func(cmd *cobra.Command, args []string) {
			if _, err := exec.LookPath("gh"); err != nil {
				die("install 'gh'")
			}

			if markRead {
				if exclusion != "" || filter != "" {
					die("Can't mark all notifications as read when either the '--exclude' or '--filter' flag was used, as it would also mark notifications as read that are filtered out.")
				}
				if err := markAllRead(isoTime()); err != nil {
					die("Failed to mark notifications as read.")
				}
				fmt.Println("All notifications have been marked as read.")
				os.Exit(0)
			}

			notifs, err := getNotifications(numNotifications, onlyParticipating, includeAll, exclusion, filter)
			if err != nil {
				die(err.Error())
			}
			if len(notifs) == 0 {
				fmt.Println(finalMsg)
				os.Exit(0)
			}
			if printStatic {
				for _, n := range notifs {
					fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s/%s\t%s\t%s\t%s\t%s\n",
						shortDate(n.UpdatedAt), isoTime(), n.ID,
						func() string {
							if n.Unread {
								return "UNREAD"
							} else {
								return "READ"
							}
						}(),

						lastPathComponent(n.Subject.LatestCommentURL), n.Repository.FullName,
						func() string {
							if n.Unread {
								return "●"
							} else {
								return " "
							}
						}(),

						abbreviate(n.Repository.Owner.Login, 10), abbreviate(n.Repository.Name, 13),
						n.Subject.Type, n.Subject.URL, n.Reason, n.Subject.Title)
				}
				os.Exit(0)
			}
			p := tea.NewProgram(NewModel(notifs), tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				die(fmt.Sprintf("Bubbletea error: %v", err))
			}
		},
	}

	rootCmd.Flags().StringVarP(&exclusion, "exclude", "e", "", "exclude notifications matching a string (REGEX support)")
	rootCmd.Flags().StringVarP(&filter, "filter", "f", "", "filter notifications matching a string (REGEX support)")
	rootCmd.Flags().IntVarP(&numNotifications, "num", "n", ghNotifyPerPageLimit, "max number of notifications to show")
	rootCmd.Flags().StringVarP(&updateSubscriptionURL, "url", "u", "", "(un)subscribe a URL, useful for issues/prs of interest")
	rootCmd.Flags().BoolVarP(&onlyParticipating, "participating", "p", false, "show only participating or mentioned notifications")
	rootCmd.Flags().BoolVarP(&includeAll, "all", "a", false, "show all (read/unread) notifications")
	rootCmd.Flags().BoolVarP(&printStatic, "static", "s", false, "print a static display")
	rootCmd.Flags().BoolVarP(&markRead, "mark-read", "r", false, "mark all notifications as read")
	rootCmd.Flags().BoolVarP(&ghNotifyVerbose, "verbose", "v", false, "enable verbose logging")

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printHelpText(cmd)
	})

	if err := rootCmd.Execute(); err != nil {
		die(err.Error())
	}
}

// getNotifications fetches and filters notifications for Bubbletea model
func getNotifications(numNotifications int, onlyParticipating, includeAll bool, exclusion, filter string) ([]Notification, error) {
	pageNum := 1
	fetchedCount := 0
	var allNotifs []Notification
	for {
		notifs, err := getNotifs(pageNum, onlyParticipating, includeAll)
		if err != nil {
			return nil, err
		}
		if len(notifs) == 0 {
			break
		}
		pageSize := min(numNotifications-fetchedCount, ghNotifyPerPageLimit)
		if pageSize < len(notifs) {
			notifs = notifs[:pageSize]
		}
		for _, n := range notifs {
			if exclusion != "" && strings.Contains(n.Subject.Title, exclusion) {
				continue
			}
			if filter != "" && !strings.Contains(n.Subject.Title, filter) {
				continue
			}
			allNotifs = append(allNotifs, n)
		}
		fetchedCount += len(notifs)
		if fetchedCount == numNotifications || len(notifs) < ghNotifyPerPageLimit {
			break
		}
		pageNum++
	}
	return allNotifs, nil
}

type Model struct {
	notifications []Notification
	cursor        int
	width         int
	height        int
	showPreview   bool
	showHelp      bool
}

func NewModel(notifs []Notification) Model {
	return Model{
		notifications: notifs,
		cursor:        0,
		showPreview:   false,
		showHelp:      false,
	}
}

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57")).Bold(true)
	unreadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	readStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true).Underline(true)
	previewStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Padding(1, 2)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("0"))
)

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.notifications)-1 {
				m.cursor++
			}
		case "tab":
			m.showPreview = !m.showPreview
		case "?":
			m.showHelp = !m.showHelp
		case "enter":
			m.showPreview = true
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	// Sticky header block
	headerLines := []string{
		headerStyle.Render("GitHub Notifications"),
	}
	maxIdx := len(m.notifications)
	idxDigits := len(fmt.Sprintf("%d", maxIdx))
	idxWidth := idxDigits + 2 // cursor + digits
	repoWidth := 22
	typeWidth := 12
	reasonWidth := 24
	stateWidth := 7
	titleWidth := max(m.width-idxWidth-repoWidth-typeWidth-reasonWidth-stateWidth-10, 8)

	header := fmt.Sprintf("%-*s %-*s%-*s%-*s%-*s%-*s",
		idxWidth, "Idx",
		repoWidth, "Repo",
		typeWidth, "Type",
		reasonWidth, "Reason",
		titleWidth, "Title",
		stateWidth, "State",
	)
	headerLines = append(headerLines, headerStyle.Render(header))
	stickyHeader := strings.Join(headerLines, "\n") + "\n"

	b.WriteString(stickyHeader)

	// Calculate available height for entries
	entriesHeight := m.height - len(headerLines) - 1
	start := 0
	end := len(m.notifications)
	if entriesHeight > 0 && end > entriesHeight {
		if m.cursor < entriesHeight/2 {
			start = 0
			end = entriesHeight
		} else if m.cursor > end-entriesHeight/2 {
			start = end - entriesHeight
		} else {
			start = m.cursor - entriesHeight/2
			end = start + entriesHeight
		}
	}

	for i := start; i < end; i++ {
		n := m.notifications[i]
		cursor := "  "
		if m.cursor == i {
			cursor = "▶ "
		}
		style := readStyle
		state := "READ"
		if n.Unread {
			style = unreadStyle
			state = "UNREAD"
		}
		repo := abbreviate(n.Repository.FullName, repoWidth)
		typ := abbreviate(n.Subject.Type, typeWidth)
		reason := abbreviate(n.Reason, reasonWidth)
		title := abbreviate(n.Subject.Title, titleWidth)
		row := fmt.Sprintf("%s%-*d%-*s%-*s%-*s%-*s%-*s",
			cursor,
			idxWidth-1, i+1,
			repoWidth, repo,
			typeWidth, typ,
			reasonWidth, reason,
			titleWidth, title,
			stateWidth, state,
		)
		if m.cursor == i {
			b.WriteString(selectedStyle.Render(row))
		} else {
			b.WriteString(style.Render(row))
		}
		b.WriteString("\n")
	}

	if m.showPreview && len(m.notifications) > 0 {
		n := m.notifications[m.cursor]
		preview := fmt.Sprintf(
			"Title: %s\nRepo: %s\nType: %s\nReason: %s\nURL: %s\nLast Updated: %s\nUnread: %v\n",
			n.Subject.Title, n.Repository.FullName, n.Subject.Type, n.Reason, n.Subject.URL, n.UpdatedAt, n.Unread,
		)
		b.WriteString(previewStyle.Render(preview))
	}

	if m.showHelp {
		help := fmt.Sprintf("↑/↓: Move  %s/%s: Preview  %s: Toggle Help  q/esc: Quit", ghNotifyViewKey, ghNotifyTogglePreviewKey, ghNotifyToggleHelpKey)
		helpLine := helpStyle.Render(help)
		lines := strings.Split(b.String(), "\n")
		if len(lines) > m.height-1 {
			lines = lines[:m.height-1]
		}
		for len(lines) < m.height-1 {
			lines = append(lines, "")
		}
		lines = append(lines, helpLine)
		return strings.Join(lines, "\n")
	}

	return b.String()
}

func initConfig() {
	viper.SetConfigName("gh-notify")
	viper.SetConfigType("toml")
	viper.AddConfigPath("$HOME/.config/gh-notify")
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("GH_NOTIFY_VIEW_KEY", "enter")
	viper.SetDefault("GH_NOTIFY_TOGGLE_PREVIEW_KEY", "tab")
	viper.SetDefault("GH_NOTIFY_TOGGLE_HELP_KEY", "?")
	viper.SetDefault("GH_NOTIFY_DEBUG_MODE", false)
	viper.SetDefault("GH_NOTIFY_CACHE_ENABLED", true)
	viper.SetDefault("GH_NOTIFY_CACHE_DURATION", "5m")
	viper.SetDefault("GH_NOTIFY_VERBOSE", false)

	_ = viper.ReadInConfig()

	ghNotifyViewKey = viper.GetString("GH_NOTIFY_VIEW_KEY")
	ghNotifyTogglePreviewKey = viper.GetString("GH_NOTIFY_TOGGLE_PREVIEW_KEY")
	ghNotifyToggleHelpKey = viper.GetString("GH_NOTIFY_TOGGLE_HELP_KEY")
	ghNotifyDebugMode = viper.GetBool("GH_NOTIFY_DEBUG_MODE")
	ghNotifyVerbose = viper.GetBool("GH_NOTIFY_VERBOSE") || ghNotifyDebugMode

	cacheEnabled = viper.GetBool("GH_NOTIFY_CACHE_ENABLED")
	durStr := viper.GetString("GH_NOTIFY_CACHE_DURATION")
	dur, err := time.ParseDuration(durStr)
	if err != nil {
		dur = 5 * time.Minute
	}
	cacheDuration = dur
	xdgCache := os.Getenv("XDG_CACHE_HOME")
	if xdgCache == "" {
		xdgCache = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	cacheDir = filepath.Join(xdgCache, "gh-notify")
	os.MkdirAll(cacheDir, 0700)
}
