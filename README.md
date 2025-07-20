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

- [GitHub CLI (gh)]

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

|     Flag / Option     | Description                                             | Example                                              |
| :-------------------: | ------------------------------------------------------- | ---------------------------------------------------- |
|        <none>         | show all unread notifications                           | `gh notify`                                          |
|      `-a, --all`      | show all (read/unread) notifications                    | `gh notify -a`                                       |
|    `-e, --exclude`    | exclude notifications matching a string (REGEX support) | `gh notify -e "MyJob"`                               |
|    `-f, --filter`     | filter notifications matching a string (REGEX support)  | `gh notify -f "Repo"`                                |
|     `-h, --help`      | show the help page                                      | `gh notify -h`                                       |
|    `-n, --num NUM`    | max number of notifications to show                     | `gh notify -n 10`                                    |
| `-p, --participating` | show only participating or mentioned notifications      | `gh notify -p`                                       |
|   `-r, --mark-read`   | mark all notifications as read                          | `gh notify -r`                                       |
|    `-s, --static`     | print a static display                                  | `gh notify -s`                                       |
|    `-u, --url URL`    | (un)subscribe a URL, useful for issues/prs of interest  | `gh notify -u https://github.com/cli/cli/issues/659` |

### Configuration

You can configure `gh-notify` using either environment variables or a TOML
config file. All configuration options can be set via environment variables, or
by creating a config file named `gh-notify.toml` in
`$HOME/.config/gh-notify/gh-notify.toml` or the current working directory.

Example `gh-notify.toml`:

```kdl
GH_NOTIFY_VIEW_KEY = "enter"
GH_NOTIFY_TOGGLE_PREVIEW_KEY = "tab"
GH_NOTIFY_TOGGLE_HELP_KEY = "?"
GH_NOTIFY_DEBUG_MODE = false
```

Environment variables always override config file values.

### Key Bindings

| Key              | Description                                        | Customization Environment Variable |
| ---------------- | -------------------------------------------------- | ---------------------------------- |
| <kbd>?</kbd>     | toggle help                                        | `GH_NOTIFY_TOGGLE_HELP_KEY`        |
| <kbd>enter</kbd> | view the selected notification in the 'less' pager | `GH_NOTIFY_VIEW_KEY`               |
| <kbd>tab</kbd>   | toggle notification preview                        | `GH_NOTIFY_TOGGLE_PREVIEW_KEY`     |
| <kbd>esc</kbd>   | quit                                               |                                    |

## Customizing

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
