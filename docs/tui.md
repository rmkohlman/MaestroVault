# TUI Guide

Launch the interactive terminal UI with:

```bash
mav tui
```

The TUI provides a full-screen interface for browsing, searching, copying, editing, and generating secrets without remembering CLI flags.

## Vim Mode

Enable vim-style keybindings with:

```bash
mav tui --vim
```

Or enable permanently via the TUI settings overlay (`S`) or `~/.maestrovault/config.json`:

```json
{ "vim_mode": true }
```

Vim mode adds Normal and Visual modes with a mode indicator in the footer.

## Theme

The TUI uses ANSI palette colors (0-15), which means it automatically inherits your terminal's color theme. Whether you use Dracula, Solarized, Catppuccin, or any other theme, MaestroVault looks native.

## Screens

### Secret List (Main Screen)

The main screen shows all secrets in a scrollable list with:

- Name
- Environment badge
- Metadata (as colored key=value pairs)
- Created/updated timestamps

### Secret Detail

Press `Enter` on a secret to view its full details including the decrypted value (masked by default). Press `Space` to toggle value masking.

### Confirm Delete

Pressing `d` (simple mode) or `x`/`dd` (vim mode) on a secret opens a confirmation screen. Press `y` to confirm deletion or `n`/`Esc` to cancel.

## Overlays

### Secret Modal

The secret modal is an interactive overlay for viewing, editing, and adding secrets.

**View mode** — Opens when you press `Enter` on a secret from the list (or via `mav get` on a TTY). The value is masked with `●●●●●●●●`.

| Key | Action |
|-----|--------|
| `p` | Peek (toggle value visibility) |
| `c` | Copy value to clipboard |
| `e` | Switch to edit mode |
| `q` / `Esc` | Close modal |

**Edit mode** — Opens when you press `e` from view mode, or `e` from the list/detail screen. All fields are pre-populated and editable (including name, which allows renaming).

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate between fields |
| `Ctrl+r` | Peek (toggle value visibility) |
| `Enter` | Save changes |
| `Esc` | Cancel and return to view mode |

**Add mode** — Opens when you press `a` (simple mode) or `i`/`a`/`o` (vim mode) from the list screen. Empty fields for entering a new secret.

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Navigate between fields |
| `Ctrl+r` | Peek (toggle value visibility) |
| `Enter` | Save new secret |
| `Esc` | Close modal |

### Password Generator

Press `n` to open the password generator with configurable length, character sets, optional name, and environment.

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate options |
| `h` / `l` (or `Left` / `Right`) | Adjust length |
| `Space` | Toggle character set options |
| `r` | Regenerate password |
| `c` | Copy password to clipboard |
| `Enter` | Save (if name provided) or close |
| `Esc` | Close generator |

### Vault Info

Press `I` (capital i, vim mode only) to view vault metadata: directory, database path, size, and secret count.

### Settings

Press `S` (capital s) to open the settings overlay. Toggle options with `Space` or `Enter`:

- **Vim Mode** — enable/disable vim keybindings
- **TouchID** — enable/disable biometric authentication
- **Fuzzy Search** — enable/disable fuzzy matching in search

Settings are saved to `~/.maestrovault/config.json` when the overlay is closed.

### Help Overlay

Press `?` to see all available keybindings.

## Keyboard Shortcuts

### List Screen (Simple Mode)

| Key | Action |
|-----|--------|
| `j` / `Down` | Move down |
| `k` / `Up` | Move up |
| `Enter` | View secret detail |
| `/` | Start search/filter |
| `a` | Add new secret (opens modal) |
| `e` | Edit selected secret |
| `c` | Copy secret value to clipboard |
| `d` | Delete secret (with confirmation) |
| `n` | Open password generator |
| `s` | Cycle sort order |
| `r` | Refresh/reload secrets |
| `S` | Open settings |
| `?` | Toggle help overlay |
| `Esc` | Back / close |
| `q` | Quit |

### List Screen (Vim Normal Mode)

| Key | Action |
|-----|--------|
| `j` / `Down` | Move down |
| `k` / `Up` | Move up |
| `g` `g` | Jump to top |
| `G` | Jump to bottom |
| `Ctrl+d` | Page down |
| `Ctrl+u` | Page up |
| `Enter` / `l` | View secret detail |
| `/` | Start search/filter |
| `i` / `a` / `o` | Add new secret (opens modal) |
| `e` | Edit selected secret |
| `c` | Copy secret value to clipboard |
| `x` | Delete secret (with confirmation) |
| `d` `d` | Delete secret (vim-style) |
| `v` / `V` | Enter visual mode |
| `n` | Open password generator |
| `I` | Show vault info |
| `S` | Open settings |
| `s` | Cycle sort order |
| `r` | Refresh/reload secrets |
| `?` | Toggle help overlay |
| `Esc` | Back / close |
| `q` | Quit |

### Detail Screen

| Key | Action |
|-----|--------|
| `Space` | Toggle value masking (show/hide) |
| `c` | Copy value to clipboard |
| `e` | Edit secret (opens modal) |
| `d` | Delete secret (with confirmation) |
| `Esc` / `h` | Back to list |
| `q` | Quit |

### Sort Orders

Pressing `s` cycles through four sort orders:

1. **Name A-Z** (default)
2. **Name Z-A**
3. **Newest first**
4. **Oldest first**

### Search

| Key | Action |
|-----|--------|
| `/` | Activate search bar |
| `Enter` | Confirm search |
| `Esc` | Cancel search |

The search filters secrets by name, environment, and metadata text in real time. Enable fuzzy matching in settings for approximate matches.

### Toast Notifications

Actions like copy, delete, and edit show brief toast notifications at the bottom of the screen confirming the operation.

## Footer Status Bar

The footer shows:

- Current mode (vim mode: NORMAL / VISUAL)
- Secret count and filter status
- Sort order indicator
- Keyboard hints for available actions
