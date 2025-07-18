package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/fatih/color"
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
	green     = color.New(color.FgGreen).SprintFunc()
	darkGray  = color.New(color.FgHiBlack).SprintFunc()
	whiteBold = color.New(color.FgWhite, color.Bold).SprintFunc()
)

var (
	ghNotifyMarkAllReadKey,
	ghNotifyOpenBrowserKey,
	ghNotifyViewDiffKey,
	ghNotifyViewPatchKey,
	ghNotifyReloadKey,
	ghNotifyMarkReadKey,
	ghNotifyCommentKey,
	ghNotifyToggleKey,
	ghNotifyResizePreviewKey,
	ghNotifyViewKey,
	ghNotifyTogglePreviewKey,
	ghNotifyToggleHelpKey string
	ghNotifyDebugMode bool
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

func printHelpText() {
	fmt.Printf("%sUsage%s\n  gh notify [Flags]\n\n", whiteBold(""), "")
	fmt.Printf("%sFlags%s\n", whiteBold(""), "")
	fmt.Printf("  %s<none>%s  show all unread notifications\n", green(""), "")
	fmt.Printf("  %s-a    %s  show all (read/ unread) notifications\n", green(""), "")
	fmt.Printf("  %s-e    %s  exclude notifications matching a string (REGEX support)\n", green(""), "")
	fmt.Printf("  %s-f    %s  filter notifications matching a string (REGEX support)\n", green(""), "")
	fmt.Printf("  %s-h    %s  show the help page\n", green(""), "")
	fmt.Printf("  %s-n NUM%s  max number of notifications to show\n", green(""), "")
	fmt.Printf("  %s-p    %s  show only participating or mentioned notifications\n", green(""), "")
	fmt.Printf("  %s-r    %s  mark all notifications as read\n", green(""), "")
	fmt.Printf("  %s-s    %s  print a static display\n", green(""), "")
	fmt.Printf("  %s-u URL%s  (un)subscribe a URL, useful for issues/prs of interest\n", green(""), "")
	fmt.Printf("  %s-w    %s  display the preview window in interactive mode\n\n", green(""), "")
	fmt.Printf("%sKey Bindings fzf%s\n", whiteBold(""), "")
	fmt.Printf("  %s%s%s        toggle help\n", green(""), ghNotifyToggleHelpKey, "")
	fmt.Printf("  %s%s%s    view the selected notification in the 'less' pager\n", green(""), ghNotifyViewKey, "")
	fmt.Printf("  %s%s%s      toggle notification preview\n", green(""), ghNotifyTogglePreviewKey, "")
	fmt.Printf("  %s%s%s       resize the preview window\n", green(""), ghNotifyResizePreviewKey, "")
	fmt.Printf("  %sshift+↑↓ %s  scroll the preview up/ down\n", green(""), "")
	fmt.Printf("  %s%s%s   mark all displayed notifications as read and reload\n", green(""), ghNotifyMarkAllReadKey, "")
	fmt.Printf("  %s%s%s   browser\n", green(""), ghNotifyOpenBrowserKey, "")
	fmt.Printf("  %s%s%s   view diff\n", green(""), ghNotifyViewDiffKey, "")
	fmt.Printf("  %s%s%s   view diff in patch format\n", green(""), ghNotifyViewPatchKey, "")
	fmt.Printf("  %s%s%s   reload\n", green(""), ghNotifyReloadKey, "")
	fmt.Printf("  %s%s%s   mark the selected notification as read and reload\n", green(""), ghNotifyMarkReadKey, "")
	fmt.Printf("  %s%s%s   write a comment with the editor and quit\n", green(""), ghNotifyCommentKey, "")
	fmt.Printf("  %s%s%s   toggle the selected notification\n", green(""), ghNotifyToggleKey, "")
	fmt.Printf("  %sesc      %s  quit\n\n", green(""), "")
	fmt.Printf("%sTable Format%s\n", whiteBold(""), "")
	fmt.Printf("  %sunread symbol%s  indicates unread status\n", green(""), "")
	fmt.Printf("  %stime         %s  time of last read for unread; otherwise, time of last update\n", green(""), "")
	fmt.Printf("  %srepo         %s  related repository\n", green(""), "")
	fmt.Printf("  %stype         %s  notification type\n", green(""), "")
	fmt.Printf("  %snumber       %s  associated number\n", green(""), "")
	fmt.Printf("  %sreason       %s  trigger reason\n", green(""), "")
	fmt.Printf("  %stitle        %s  notification title\n\n", green(""), "")
	fmt.Printf("%sExample%s\n", whiteBold(""), "")
	fmt.Printf("    %s# Display the last 20 notifications%s\n    gh notify -an 20\n", darkGray(""), "")
}

func checkVersion(tool, threshold string) {
	out, err := exec.Command(tool, "--version").Output()
	if err != nil {
		die(fmt.Sprintf("Your '%s' version is insufficient. The minimum required version is '%s'.", tool, threshold))
	}
	re := regexp.MustCompile(`[0-9]+(\.[0-9]+)*`)
	userVersion := re.FindString(string(out))
	verParts := strings.Split(userVersion, ".")
	threshParts := strings.Split(threshold, ".")
	for i := range threshParts {
		user, _ := strconv.Atoi(getOrDefault(verParts, i, "0"))
		thresh, _ := strconv.Atoi(getOrDefault(threshParts, i, "0"))
		if user < thresh {
			die(fmt.Sprintf("Your '%s' version '%s' is insufficient. The minimum required version is '%s'.", tool, userVersion, threshold))
		}
		if user > thresh {
			break
		}
	}
}

func getOrDefault(parts []string, idx int, def string) string {
	if idx < len(parts) {
		return parts[idx]
	}
	return def
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

func getNotifs(pageNum int, onlyParticipating, includeAll bool) ([]Notification, error) {
	client := ghRestApiClient()
	var notifs []Notification
	endpoint := fmt.Sprintf("notifications?per_page=%d&page=%d&participating=%t&all=%t",
		ghNotifyPerPageLimit, pageNum, onlyParticipating, includeAll)
	if err := client.Get(endpoint, &notifs); err != nil {
		return nil, err
	}
	return notifs, nil
}

func printNotifs(numNotifications int, onlyParticipating, includeAll bool, exclusion, filter string) (string, error) {
	pageNum := 1
	fetchedCount := 0
	var allNotifs []Notification
	for {
		notifs, err := getNotifs(pageNum, onlyParticipating, includeAll)
		if err != nil {
			return "", err
		}
		if len(notifs) == 0 {
			break
		}
		pageSize := min(numNotifications-fetchedCount, ghNotifyPerPageLimit)
		if pageSize < len(notifs) {
			notifs = notifs[:pageSize]
		}
		allNotifs = append(allNotifs, notifs...)
		fetchedCount += len(notifs)
		if fetchedCount == numNotifications || len(notifs) < ghNotifyPerPageLimit {
			break
		}
		pageNum++
	}
	var buf bytes.Buffer
	for _, n := range allNotifs {
		if exclusion != "" && strings.Contains(n.Subject.Title, exclusion) {
			continue
		}
		if filter != "" && !strings.Contains(n.Subject.Title, filter) {
			continue
		}
		updatedShort := shortDate(n.UpdatedAt)
		iso8601 := isoTime()
		threadState := "READ"
		if n.Unread {
			threadState = "UNREAD"
		}
		commentURL := lastPathComponent(n.Subject.LatestCommentURL)
		repoFullName := n.Repository.FullName
		unreadSymbol := "●"
		if !n.Unread {
			unreadSymbol = " "
		}
		ownerAbbr := abbreviate(n.Repository.Owner.Login, 10)
		nameAbbr := abbreviate(n.Repository.Name, 13)
		typ := n.Subject.Type
		url := n.Subject.URL
		reason := n.Reason
		title := n.Subject.Title
		buf.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s/%s\t%s\t%s\t%s\t%s\n",
			updatedShort, iso8601, n.ID, threadState, commentURL, repoFullName,
			unreadSymbol, ownerAbbr, nameAbbr, typ, url, reason, title))
	}
	return buf.String(), nil
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
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

func timeAgo(lastRead, updated string, unread bool) string {
	var t time.Time
	var err error
	if unread && lastRead != "" {
		t, err = time.Parse(time.RFC3339, lastRead)
	} else {
		t, err = time.Parse(time.RFC3339, updated)
	}
	if err != nil {
		return "Not available"
	}
	diff := time.Since(t)
	switch {
	case diff < time.Hour:
		return fmt.Sprintf("%dmin ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return t.Format("02/Jan 15:04")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func markAllRead(isoTime string) error {
	client := ghRestApiClient()
	body := map[string]interface{}{
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
		printStatic, markRead, showHelp          bool
	)
	flag.StringVar(&exclusion, "e", "", "")
	flag.StringVar(&filter, "f", "", "")
	flag.IntVar(&numNotifications, "n", ghNotifyPerPageLimit, "")
	flag.StringVar(&updateSubscriptionURL, "u", "", "")
	flag.BoolVar(&onlyParticipating, "p", false, "")
	flag.BoolVar(&includeAll, "a", false, "")
	flag.BoolVar(&printStatic, "s", false, "")
	flag.BoolVar(&markRead, "r", false, "")
	flag.BoolVar(&showHelp, "h", false, "")
	flag.Parse()

	if showHelp {
		printHelpText()
		os.Exit(0)
	}

	if _, err := exec.LookPath("gh"); err != nil {
		die("install 'gh'")
	}

	if markRead {
		if exclusion != "" || filter != "" {
			die("Can't mark all notifications as read when either the '-e' or '-f' flag was used, as it would also mark notifications as read that are filtered out.")
		}
		if err := markAllRead(isoTime()); err != nil {
			die("Failed to mark notifications as read.")
		}
		fmt.Println("All notifications have been marked as read.")
		os.Exit(0)
	}

	if !printStatic {
		if _, err := exec.LookPath("fzf"); err != nil {
			die("install 'fzf' or use the -s flag")
		}
		checkVersion("fzf", minFzfVersion)
	}

	notifs, err := printNotifs(numNotifications, onlyParticipating, includeAll, exclusion, filter)
	if err != nil {
		die(err.Error())
	}
	if notifs == "" {
		fmt.Println(finalMsg)
		os.Exit(0)
	}
	if printStatic {
		for _, line := range strings.Split(notifs, "\n") {
			cols := strings.Split(line, "\t")
			if len(cols) > 6 {
				fmt.Println(strings.Join(cols[6:], "\t"))
			}
		}
		os.Exit(0)
	}
	runFzf(notifs)
}

func runFzf(notifs string) {
	cmd := exec.Command("fzf",
		"--ansi",
		"--bind", fmt.Sprintf("%s:change-preview-window(75%%:nohidden|75%%:down:nohidden:border-top|nohidden)", ghNotifyResizePreviewKey),
		"--bind", "change:first",
		"--bind", fmt.Sprintf("%s:select-all+reload:echo reload", ghNotifyMarkAllReadKey),
		"--bind", fmt.Sprintf("%s:execute-silent:echo browser", ghNotifyOpenBrowserKey),
		"--bind", fmt.Sprintf("%s:toggle-preview+change-preview:echo diff", ghNotifyViewDiffKey),
		"--bind", fmt.Sprintf("%s:toggle-preview+change-preview:echo patch", ghNotifyViewPatchKey),
		"--bind", fmt.Sprintf("%s:reload:echo reload", ghNotifyReloadKey),
		"--bind", fmt.Sprintf("%s:execute-silent:echo mark", ghNotifyMarkReadKey),
		"--bind", fmt.Sprintf("%s:toggle+down", ghNotifyToggleKey),
		"--bind", fmt.Sprintf("%s:execute:echo pager", ghNotifyViewKey),
		"--bind", fmt.Sprintf("%s:toggle-preview+change-preview:echo preview", ghNotifyTogglePreviewKey),
		"--bind", fmt.Sprintf("%s:toggle-preview+change-preview:echo help", ghNotifyToggleHelpKey),
		"--border", "horizontal",
		"--color", "border:dim",
		"--color", "header:green:italic:dim",
		"--color", "prompt:80,info:40",
		"--delimiter", "\\s+",
		"--expect", fmt.Sprintf("esc,%s", ghNotifyCommentKey),
		"--header", fmt.Sprintf("%s help · esc quit", ghNotifyToggleHelpKey),
		"--info=inline",
		"--multi",
		"--pointer=▶",
		"--preview", "echo preview",
		"--preview-window", "default:wrap:hidden:60%:right:border-left",
		"--no-print-query",
		"--prompt", "GitHub Notifications > ",
		"--reverse",
		"--with-nth", "6..",
	)
	cmd.Stdin = strings.NewReader(notifs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil && !errors.Is(err, os.ErrClosed) {
		die(fmt.Sprintf("fzf error: %v", err))
	}
}

func initConfig() {
	viper.SetConfigName("gh-notify")
	viper.SetConfigType("toml")
	viper.AddConfigPath("$HOME/.config/gh-notify")
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("GH_NOTIFY_MARK_ALL_READ_KEY", "ctrl-a")
	viper.SetDefault("GH_NOTIFY_OPEN_BROWSER_KEY", "ctrl-b")
	viper.SetDefault("GH_NOTIFY_VIEW_DIFF_KEY", "ctrl-d")
	viper.SetDefault("GH_NOTIFY_VIEW_PATCH_KEY", "ctrl-p")
	viper.SetDefault("GH_NOTIFY_RELOAD_KEY", "ctrl-r")
	viper.SetDefault("GH_NOTIFY_MARK_READ_KEY", "ctrl-t")
	viper.SetDefault("GH_NOTIFY_COMMENT_KEY", "ctrl-x")
	viper.SetDefault("GH_NOTIFY_TOGGLE_KEY", "ctrl-y")
	viper.SetDefault("GH_NOTIFY_RESIZE_PREVIEW_KEY", "btab")
	viper.SetDefault("GH_NOTIFY_VIEW_KEY", "enter")
	viper.SetDefault("GH_NOTIFY_TOGGLE_PREVIEW_KEY", "tab")
	viper.SetDefault("GH_NOTIFY_TOGGLE_HELP_KEY", "?")
	viper.SetDefault("GH_NOTIFY_DEBUG_MODE", false)

	_ = viper.ReadInConfig()

	ghNotifyMarkAllReadKey = viper.GetString("GH_NOTIFY_MARK_ALL_READ_KEY")
	ghNotifyOpenBrowserKey = viper.GetString("GH_NOTIFY_OPEN_BROWSER_KEY")
	ghNotifyViewDiffKey = viper.GetString("GH_NOTIFY_VIEW_DIFF_KEY")
	ghNotifyViewPatchKey = viper.GetString("GH_NOTIFY_VIEW_PATCH_KEY")
	ghNotifyReloadKey = viper.GetString("GH_NOTIFY_RELOAD_KEY")
	ghNotifyMarkReadKey = viper.GetString("GH_NOTIFY_MARK_READ_KEY")
	ghNotifyCommentKey = viper.GetString("GH_NOTIFY_COMMENT_KEY")
	ghNotifyToggleKey = viper.GetString("GH_NOTIFY_TOGGLE_KEY")
	ghNotifyResizePreviewKey = viper.GetString("GH_NOTIFY_RESIZE_PREVIEW_KEY")
	ghNotifyViewKey = viper.GetString("GH_NOTIFY_VIEW_KEY")
	ghNotifyTogglePreviewKey = viper.GetString("GH_NOTIFY_TOGGLE_PREVIEW_KEY")
	ghNotifyToggleHelpKey = viper.GetString("GH_NOTIFY_TOGGLE_HELP_KEY")
	ghNotifyDebugMode = viper.GetBool("GH_NOTIFY_DEBUG_MODE")
}
