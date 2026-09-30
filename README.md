# asp — Agent Session Picker

A terminal UI for browsing, naming, grouping, resuming and starting
**Claude Code** and **Codex CLI** sessions, in one list. Sessions run *under*
asp: press `ctrl+z` inside Claude or Codex to pause it and come back to the
list, and `↵` on it to carry on exactly where you were.

## Requirements

To build:

- **Go 1.27+**. The Go libraries asp uses (Bubble Tea, Lip Gloss, Bubbles,
  `x/ansi`, `x/sys`, `atotto/clipboard`, `go-osc52`, `BurntSushi/toml`) are downloaded
  automatically by `go build`; see `go.mod`.
- `make` (optional; `go build -o asp .` does the same).

To run:

- **Linux.** Running agents under asp uses POSIX job control; the per-thread
  signal handling it needs is Linux-only for now. On other systems asp still
  lists, names and groups sessions but cannot open them.
- **Claude Code** (`claude`) and/or **Codex CLI** (`codex`) on your `PATH`.
  Either one is enough: new sessions use whichever is installed. With
  neither, asp starts and shows any sessions already on disk (none on a
  fresh machine).
- A terminal with Unicode and 24-bit colour (kitty, foot, alacritty, …).
- For `y` (copy): `wl-copy` on Wayland, `xclip` or `xsel` on X11. Without
  them asp asks the terminal to set the clipboard itself (OSC 52), which
  kitty supports.

## Install

```sh
go install github.com/summant/asp@latest
```

That puts `asp` in `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure
that is on your `PATH`, or install straight into `~/.local/bin`:

```sh
GOBIN=~/.local/bin go install github.com/summant/asp@latest
```

Go fetches every library asp needs by itself.

### Updating

Run the same `go install` command again. That replaces only the `asp`
program: your names, groups, colours, tabs and config live in
`~/.config/asp` and are never touched by an update. After updating:

```sh
asp --update-config   # add any new settings to your config.toml (optional)
```

It adds each new setting with its default and a comment, changes nothing
you have set, and keeps the old file as `config.toml.bak`. Without it, new
settings simply use their defaults.

asp is built to never lose that data:

- Files written by earlier versions are read as they are (the tests keep
  copies of each version's formats).
- A file asp cannot read — say, after a bad edit by hand — is reported
  and left exactly as it is; asp never writes over it.
- Keys you set always win: if a new version gives a new action a default
  key you already use, the new action gives it up. Config mistakes are
  warnings, never a reason not to start (`asp --check-config` lists them).

From a clone:

```sh
make install   # builds and writes ~/.local/bin/asp — nothing else
```

`make build`, `make test`, `make fmt` and `make vet` do what they say.

## Use

```sh
asp            # the picker
asp --list     # tab-separated: agent, id, modified, messages, folder, name, groups, opening
asp --version
asp --write-config   # write ~/.config/asp/config.toml with every default, commented
asp --update-config  # add settings a newer asp introduced, keeping yours
asp --check-config   # list problems in the config
asp --config FILE    # use another config file
```

| key | action |
|---|---|
| `↵` | open the session — or go back to it if it is paused |
| `ctrl+z` | *inside Claude or Codex:* pause it and return to asp |
| `c` | end the selected paused session without going into it (it can be resumed later) |
| `n` | new session: choose the agent, name it, pick a folder |
| `/` | filter; `f:folder` and `g:group` narrow to one field, `"quoted words"` keep spaces |
| `f` | browse folders and show the sessions in the one you choose |
| `g` / `G` | add to a group (a new group asks for its colour) / remove from a group |
| `p` | recolour a group of the selected session |
| `r` / `x` | rename / clear the name |
| `v` / `y` | everything about the session, full width / copy the opening message |
| `←` `→` / `a` `d` | switch tabs |
| `+` / `-` | add a tab — claude, codex, or a group (up to 5 groups) / close the current tab (all stays) |
| `j` `k` / `↓` `↑` | move |
| `h` `l` / `pgup` `pgdn` | previous / next page |
| `home` `end` | first / last |
| `?` | all keys, as configured |
| `q` / `ctrl+c` | quit — asks first if sessions are paused, since quitting ends them. `esc` never quits |

The folder browser shows one folder at a time, like glow's file list:
type to narrow what is in it, `↑` `↓` to move, `↵` or `→` to go into a
folder, `←` or backspace to go back up. Choose with **Use this folder** at
the top, or `tab` on a highlighted folder. The right pane previews a
folder's contents or the start of a text file. `ctrl+o` opens it from the
new-session folder prompt and from the filter. In the folder prompt itself,
`↑` `↓` pick a suggestion and `tab` fills it in; with nothing typed, the
folders you have used before are listed.

Confirmations and errors appear on the line above the key hints and fade
after a few seconds.

Tabs sit in the header: `all` always, then claude, codex and up to five
groups, in the order you add them. Any tab but `all` can be closed with `-`
and brought back with `+`. A group tab lists that group's sessions, and a
session started from it joins the group. asp reopens on the tab it was
closed on.

Each group has its own colour, used for its `#tags` on sessions. A new group
asks for one: type a standard colour name (`pink`, `teal`, `hot pink`, … with
suggestions) or `#` and a hex code. Skip it and the group is gray. `p`
changes it later.

Paused sessions are shown with `paused` in the list and a count in the
header. Quitting asp ends them one at a time the way closing a terminal
would, waiting for each to finish; Claude prints the `claude --resume …`
line for each as it goes, and everything said so far can be resumed.

## Configuration

Colours and keys live in `~/.config/asp/config.toml` (or
`$XDG_CONFIG_HOME/asp/config.toml`). `asp --write-config` writes one with
every default and what it does; change what you like and delete the rest.

```toml
[colors]
accent = "#ff79c6"        # "#rrggbb", "#rgb" or an ANSI number 0-255
text   = "#e0e0e0"

[keys.list]
quit    = ["Q"]           # q no longer quits
details = ["i", "v"]

[keys.prompt]
cancel  = ["esc", "ctrl+g"]
```

Colour roles: `accent`, `accent_muted`, `secondary`, `text`, `text_strong`,
`muted`, `subtle`, `rule`, `dot_active`, `error`, and `group` (the gray used for groups without a colour). Key sections: `list`,
`prompt` (filter, rename, new-session name and folder, groups), `agent`
(choosing claude or codex), `finder` (the folder browser) and `details`
(the `v` and `?` pages). The footer, prompts and `?` show whatever you
configure. Unknown settings, bad colours, unknown key names and a key bound
twice in one section are reported when asp starts. `ctrl+c` always quits.

## Where things live

- `~/.config/asp/config.toml` — colours and keys (optional).
- `~/.config/asp/names.json` — session names, `{"<agent>:<id>": "name"}`. On
  first run it imports `~/.claude/session-names.json` from the old prototype,
  which is only ever read.
- `~/.config/asp/groups.json` — groups, `{"group": ["<agent>:<id>", …]}`.
- `~/.config/asp/group-colors.json` — each group's colour.
- `~/.config/asp/state.json` — your tabs, the one asp was closed on, and the last selected session.

asp only reads agent data (`~/.claude/projects/`, `~/.codex/sessions/`,
`~/.codex/archived_sessions/`); it never writes there.
