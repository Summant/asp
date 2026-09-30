# asp — Agent Session Picker

A terminal UI for browsing, naming, grouping, resuming and starting
**Claude Code** and **Codex CLI** sessions, in one list. Sessions run *under*
asp: press `ctrl+z` inside Claude or Codex to pause it and come back to the
list, and `↵` on it to carry on exactly where you were.

## Requirements

To build:

- **Go 1.27+**. The Go libraries asp uses (Bubble Tea, Lip Gloss, Bubbles,
  `x/ansi`, `x/sys`, `atotto/clipboard`, `go-osc52`) are downloaded
  automatically by `go build`; see `go.mod`.
- `make` (optional; `go build -o asp .` does the same).

To run:

- **Linux.** Running agents under asp uses POSIX job control; the per-thread
  signal handling it needs is Linux-only for now. On other systems asp still
  lists, names and groups sessions but cannot open them.
- **Claude Code** (`claude`) and/or **Codex CLI** (`codex`) on your `PATH`.
  Either one is enough; the other's sessions simply don't appear.
- A terminal with Unicode and 24-bit colour (kitty, foot, alacritty, …).
- For `y` (copy): `wl-copy` on Wayland, `xclip` or `xsel` on X11. Without
  them asp asks the terminal to set the clipboard itself (OSC 52), which
  kitty supports.

## Install

```sh
make install   # builds and writes ~/.local/bin/asp — nothing else
```

`make build`, `make test`, `make fmt` and `make vet` do what they say.

## Use

```sh
asp            # the picker
asp --list     # tab-separated: agent, id, modified, messages, folder, name, groups, opening
asp --version
```

| key | action |
|---|---|
| `↵` | open the session — or go back to it if it is paused |
| `ctrl+z` | *inside Claude or Codex:* pause it and return to asp |
| `n` | new session: choose the agent, name it, pick a folder |
| `/` | filter; `f:folder` and `g:group` narrow to one field, `"quoted words"` keep spaces |
| `f` | browse folders and show the sessions in the one you choose |
| `b` / `B` | add to a group / remove from a group |
| `r` / `x` | rename / clear the name |
| `v` / `y` | everything about the session, full width / copy the opening message |
| `←` `→` / `a` `d` | show all sessions, Claude only, or Codex only (remembered) |
| `j` `k` / `↓` `↑` | move |
| `h` `l` / `pgup` `pgdn` | previous / next page |
| `g` `G` | first / last |
| `?` | all keys |
| `q` | quit — asks first if sessions are paused, since quitting ends them |

The folder browser shows one folder at a time, like glow's file list:
type to narrow what is in it, `↑` `↓` to move, `↵` or `→` to go into a
folder, `←` or backspace to go back up. Choose with **Use this folder** at
the top, or `tab` on a highlighted folder. The right pane previews a
folder's contents or the start of a text file. `ctrl+o` opens it from the
new-session folder prompt and from the filter. In the folder prompt itself,
`↑` `↓` pick a suggestion and `tab` fills it in; with nothing typed, the
folders you have used before are listed.

Paused sessions are shown with `paused` in the list and a count in the
header. Quitting asp ends them one at a time the way closing a terminal
would, waiting for each to finish; Claude prints the `claude --resume …`
line for each as it goes, and everything said so far can be resumed.

## Where things live

- `~/.config/asp/names.json` — session names, `{"<agent>:<id>": "name"}`. On
  first run it imports `~/.claude/session-names.json` from the old prototype,
  which is only ever read.
- `~/.config/asp/groups.json` — groups, `{"group": ["<agent>:<id>", …]}`.
- `~/.config/asp/state.json` — the last view and selected session.

asp only reads agent data (`~/.claude/projects/`, `~/.codex/sessions/`,
`~/.codex/archived_sessions/`); it never writes there.
