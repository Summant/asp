# asp — Agent Session Picker

A terminal UI for browsing, naming, resuming and starting **Claude Code** and
**Codex CLI** sessions, in one list.

## Install

```sh
make install   # builds and writes ~/.local/bin/asp — nothing else
```

`make build`, `make test`, `make fmt` and `make vet` do what they say.

## Use

```sh
asp            # the picker
asp --list     # tab-separated: agent, id, modified, messages, folder, name, opening
asp --version
```

| key | action |
|---|---|
| `↵` | resume the selected session |
| `n` | new session: name, then folder (`tab` in the name prompt switches agent, `tab` in the folder prompt completes) |
| `r` / `x` | rename / clear the name |
| `/` | filter (`enter` keeps it, `esc` clears it) |
| `←` `→` / `a` `d` | show all sessions, Claude only, or Codex only |
| `j` `k` / `↓` `↑` | move |
| `h` `l` / `pgup` `pgdn` | previous / next page |
| `g` `G` | first / last |
| `q` `esc` `ctrl+c` | quit |

A new session starts with the agent of the current view (Claude in the "all"
view).

## Where things live

- `~/.config/asp/names.json` — your session names, `{"<agent>:<id>": "name"}`.
  On first run it imports `~/.claude/session-names.json` from the old
  prototype, which is only ever read.
- `~/.config/asp/state.json` — the last view and selected session.

asp only reads agent data (`~/.claude/projects/`, `~/.codex/sessions/`,
`~/.codex/archived_sessions/`); it never writes there.
