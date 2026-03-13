# TUI Guide

Launch the interactive terminal UI with:

```bash
maestrovault tui
```

The TUI provides a full-screen interface for browsing, searching, copying, editing, and generating secrets without remembering CLI flags.

## Vim Mode

Enable vim-style keybindings with:

```bash
maestrovault tui --vim
```

Vim mode adds Normal, Visual, and Insert modes with a mode indicator in the footer.

## Theme

The TUI uses ANSI palette colors (0-15), which means it automatically inherits your terminal's color theme. Whether you use Dracula, Solarized, Catppuccin, or any other theme, MaestroVault looks native.

## Screens

### Secret List (Main Screen)

The main screen shows all secrets in a scrollable list with:

- Name
- Labels (as colored key=value pairs)
- Created/updated timestamps

### Secret Detail

Press `Enter` on a secret to view its full details including the decrypted value (masked by default).

### Password Generator Modal

Press `g` to open the password generator with configurable length and character sets.

### Vault Info Modal

Press `i` to view vault metadata: directory, database path, size, and secret count.

### Help Overlay

Press `?` to see all available keybindings.

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `j` / `Down` | Move down |
| `k` / `Up` | Move up |
| `g` `g` | Jump to top (vim mode) |
| `G` | Jump to bottom (vim mode) |
| `Ctrl+d` | Page down |
| `Ctrl+u` | Page up |
| `Enter` | View secret detail |
| `Esc` | Back / close modal |
| `q` | Quit |

### Actions

| Key | Action |
|-----|--------|
| `/` | Start search/filter |
| `c` | Copy secret value to clipboard |
| `e` | Edit secret in-place |
| `d` | Delete secret (with confirmation) |
| `g` | Open password generator |
| `i` | Show vault info |
| `s` | Cycle sort order (name/created/updated) |
| `r` | Reverse sort direction |
| `v` | Toggle value masking (show/hide) |
| `?` | Toggle help overlay |

### Search

| Key | Action |
|-----|--------|
| `/` | Activate search bar |
| `Enter` | Confirm search |
| `Esc` | Cancel search |

The search filters secrets by name and label text in real time.

### Toast Notifications

Actions like copy, delete, and edit show brief toast notifications at the bottom of the screen confirming the operation.

## Footer Status Bar

The footer shows:

- Current mode (vim mode: NORMAL / INSERT / VISUAL)
- Secret count and filter status
- Sort order indicator
- Keyboard hints for available actions
