package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	fmt.Printf("  %-10s  %s\n", green(ghNotifyResizePreviewKey), "resize the preview window")
	fmt.Printf("  %-10s  %s\n", green("shift+↑↓"), "scroll the preview up/ down")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyMarkAllReadKey), "mark all displayed notifications as read and reload")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyOpenBrowserKey), "browser")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyViewDiffKey), "view diff")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyViewPatchKey), "view diff in patch format")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyReloadKey), "reload")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyMarkReadKey), "mark the selected notification as read and reload")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyCommentKey), "write a comment with the editor and quit")
	fmt.Printf("  %-10s  %s\n", green(ghNotifyToggleKey), "toggle the selected notification")
	fmt.Printf("  %-10s  %s\n\n", green("esc"), "quit")
	fmt.Printf("%sTable Format%s\n", whiteBold(""), "")
	fmt.Printf("  %s  %s\n", green("unread symbol"), "indicates unread status")
	fmt.Printf("  %s  %s\n", green("time"), "time of last read for unread; otherwise, time of last update")
	fmt.Printf("  %s  %s\n", green("repo"), "related repository")
	fmt.Printf("  %s  %s\n", green("type"), "notification type")
	fmt.Printf("  %s  %s\n", green("number"), "associated number")
	fmt.Printf("  %s  %s\n", green("reason"), "trigger reason")
	fmt.Printf("  %s  %s\n\n", green("title"), "notification title")
	fmt.Printf("%sExample%s\n", whiteBold(""), "")
	fmt.Printf("    %s# Display the last 20 notifications%s\n    gh-notify --all --num 20\n", darkGray(""), "")
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
		printStatic, markRead                    bool
	)

	rootCmd := &cobra.Command{
		Use:   "gh-notify",
		Short: "GitHub notifications CLI",
		Long:  "A CLI for managing GitHub notifications.",
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

			if !printStatic {
				if _, err := exec.LookPath("fzf"); err != nil {
					die("install 'fzf' or use the --static flag")
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

	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		printHelpText(cmd)
	})

	if err := rootCmd.Execute(); err != nil {
		die(err.Error())
	}
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
