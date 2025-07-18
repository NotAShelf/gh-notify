<!-- markdownlint-disable MD013 MD033-->
<h1 id="header" align="center">
    <pre>gh-notify</pre>
</h1>

<div align="center">
    <a alt="CI" href="https://github.com/NotAShelf/gh-notify/actions">
        <img
          src="https://github.com/NotAShelf/gh-notify/actions/workflows/build.yml/badge.svg"
          alt="Build Status"
        />
    </a>
</div>

<div align="center">
  A <a alt="gh-url" href="https://github.com/cli/cli">gh</a>
  extension to view your GitHub notifications from the command line.
</div>

## Install

### Dependencies

[GitHub CLI (gh)]: https://github.com/cli/cli#installation
[Fuzzy Finder (fzf)]: https://github.com/junegunn/fzf#installation

- [GitHub CLI (gh)]
- To use `gh notify` interactively, also install [Fuzzy Finder (fzf)]. This
  allows for interaction with the listed data.

### Installing/Upgrading/Uninstalling

```sh
# Install
gh ext install NotAShelf/gh-notify

# Upgrade
gh ext upgrade NotAShelf/gh-notify

# Uninstall
gh ext remove NotAShelf/gh-notify
```

## Usage

```sh
gh notify [Flags]
```

| Flags    | Description                                             | Example                                              |
| -------- | ------------------------------------------------------- | ---------------------------------------------------- |
| <none>   | show all unread notifications                           | `gh notify`                                          |
| `-a`     | show all (read/ unread) notifications                   | `gh notify -a`                                       |
| `-e`     | exclude notifications matching a string (REGEX support) | `gh notify -e "MyJob"`                               |
| `-f`     | filter notifications matching a string (REGEX support)  | `gh notify -f "Repo"`                                |
| `-h`     | show the help page                                      | `gh notify -h`                                       |
| `-n NUM` | max number of notifications to show                     | `gh notify -an 10`                                   |
| `-p`     | show only participating or mentioned notifications      | `gh notify -ap`                                      |
| `-r`     | mark all notifications as read                          | `gh notify -r`                                       |
| `-s`     | print a static display                                  | `gh notify -an 10 -s`                                |
| `-u URL` | (un)subscribe a URL, useful for issues/prs of interest  | `gh notify -u https://github.com/cli/cli/issues/659` |
| `-w`     | display the preview window in interactive mode          | `gh notify -an 10 -w`                                |

### Configuration

You can configure `gh-notify` using either environment variables or a TOML
config file. All configuration options can be set via environment variables, or
by creating a config file named `gh-notify.toml` in one of the following
locations:

- The current working directory
- `$HOME/.config/gh-notify/gh-notify.toml`

Example `gh-notify.toml`:

```toml
GH_NOTIFY_VIEW_KEY = "enter"
GH_NOTIFY_TOGGLE_PREVIEW_KEY = "tab"
GH_NOTIFY_TOGGLE_HELP_KEY = "?"
GH_NOTIFY_DEBUG_MODE = false
```

Environment variables always override config file values.

### Key Bindings fzf

| Keys             | Description                                        | Customization Environment Variable |
| ---------------- | -------------------------------------------------- | ---------------------------------- |
| <kbd>?</kbd>     | toggle help                                        | `GH_NOTIFY_TOGGLE_HELP_KEY`        |
| <kbd>enter</kbd> | view the selected notification in the 'less' pager | `GH_NOTIFY_VIEW_KEY`               |
| <kbd>tab</kbd>   | toggle notification preview                        | `GH_NOTIFY_TOGGLE_PREVIEW_KEY`     |
| <kbd>esc</kbd>   | quit                                               |                                    |

## Customizing

### Fuzzy Finder (fzf)

[junegunn/fzf#environment-variables]: https://github.com/junegunn/fzf#environment-variables

You can customize the `fzf` key bindings by exporting `ENVIRONMENT VARIABLES` to
your `.bashrc` or `.zshrc`. For `AVAILABLE KEYS/ EVENTS`, refer to `man 1 fzf`
page or visit [junegunn/fzf#environment-variables] on GitHub.

[How to use ALT commands in a terminal on macOS?]: https://superuser.com/questions/496090/how-to-use-alt-commands-in-a-terminal-on-os-x

> [!TIP]
> If you're on MacOS, see [How to use ALT commands in a terminal on macOS?]

```bash
# ---
# In ~/.bashrc or ~/.zshrc, or alternatively their relevant XDG-spec compliant
# counterparts.
# ---
# The examples below enable you to clear the input query with alt+c,
# jump to the first/last result with alt+u/d, refresh the preview window with alt+r
# and scroll the preview in larger steps with ctrl+w/s.
export FZF_DEFAULT_OPTS="
--bind 'alt-c:clear-query'
--bind 'alt-u:first,alt-d:last'
--bind 'alt-r:refresh-preview'
--bind 'ctrl-w:preview-half-page-up,ctrl-s:preview-half-page-down'"
```

#### GH_NOTIFY_FZF_OPTS

This environment variable lets you specify additional options and key bindings
to customize the search and display of notifications. Unlike `FZF_DEFAULT_OPTS`,
`GH_NOTIFY_FZF_OPTS` specifically applies to the `gh notify` extension.

```sh
# --exact: Enables exact matching instead of fuzzy matching.
GH_NOTIFY_FZF_OPTS="--exact" gh notify -an 5
```

```sh
# With the height flag and ~, fzf adjusts its height based on input size without filling the entire screen.
# Requires fzf +0.34.0
GH_NOTIFY_FZF_OPTS="--height=~100%" gh notify -an 5
```

#### Modifying Keybindings

You can also customize the keybindings created by this extension to avoid
conflicts with the ones defined by `fzf`. For example, change `ctrl-p` to
`ctrl-u`:

```sh
GH_NOTIFY_VIEW_PATCH_KEY="ctrl-u" gh notify
```

Or, switch the binding for toggling a notification and toggling the preview.

```sh
GH_NOTIFY_TOGGLE_KEY="tab" GH_NOTIFY_TOGGLE_PREVIEW_KEY="ctrl-y" gh notify
```

> [!NOTE]
> The assigned key must be a valid key listed in the `fzf` man page:

```sh
man --pager='less -p "^\s+AVAILABLE_KEYS"' fzf
```

### GitHub Command Line Tool (gh)

In the `gh` tool's config file, you can specify your preferred editor. This is
particularly useful when you use the <kbd>ctrl</kbd><kbd>x</kbd> hotkey to
comment on a notification.

```sh
# To see more details
gh config
# For example, you can set the editor to Visual Studio Code or Vim.
gh config set editor "code --wait"
gh config set editor vim
```

## Attributions

[gh-notify]: https://github.com/meiji163/gh-notify

This repository is a structured Go port of [gh-notify], a gh extension bearing
the same name. I have elected to port it to Go for two specific reasons:

1. I am a firm believer that Bash is only suitable for _small_ scripts. As the
   size of a project grows, it becomes more unmaintainable. Go is a simple
   enough language that offsets this cost. Writing extensions when the target
   API provides a Go library also strikes me as counter-intuitive.
2. Rust API for writing gh extension seems a little immature. I also don't think
   the size of this project warrants a full Rust port, since the surface is
   relatively small.

That said, I would like to extend my thanks to the original author for their
project. Most of the logic for this project is _directly_ ported into Go,
stripping Bash idioms for Go where applicable. If upstream updates their
extension, I'll attempt to retain feature parity with full
backwards-compatibility.

## License

This project is available under [Mozilla Public License v2.0](LICENSE). Please
see the license file for more details.
