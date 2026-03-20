# MaestroVault — AI Context

> **Purpose:** macOS-first CLI secrets manager with AES-256-GCM encryption, Keychain-backed master key, and Bubble Tea TUI.
> **Work tracking:** [GitHub Project "MaestroVault"](https://github.com/users/rmkohlman/projects/2)

---

## Quick Start

1. Check GitHub Project for current sprint and open issues
2. Work in the repo root for code changes
3. All work must go through GitHub Issues — no ad-hoc changes

---

## Session Maintenance Protocol

### Session Start Protocol

At the beginning of every session, before doing any work:

1. **Check project board state**
   ```bash
   gh project item-list 2 --owner rmkohlman --format json --limit 200
   gh issue list --repo rmkohlman/MaestroVault --state open
   ```
2. **Identify interrupted work** — Items with Status = "In Progress" were interrupted from a previous session. Resume these first.
3. **Enrich any new items** — Check for items missing Agent, Effort, or Sprint fields. Set them.
   - Field IDs: Status (`PVTSSF_lAHOAr08hs4BSRTizg_2sUk`), Agent (`PVTSSF_lAHOAr08hs4BSRTizg_2sVc`), Effort (`PVTSSF_lAHOAr08hs4BSRTizg_2sVg`), Sprint (`PVTSSF_lAHOAr08hs4BSRTizg_2sVk`)
   - Status options: Todo=`f75ad846`, In Progress=`47fc9ee4`, Done=`98236657`
   - Agent options: vault-core=`5ebb3a65`, tui=`f56726be`, security=`0dc8f938`, test=`0831f615`, document=`bf3d05f2`, release=`f5532567`
   - Effort options: S=`5a2225fe`, M=`e56098de`, L=`4759f1dc`
   - Sprint options: Sprint 1=`7fa5578f`
   - Project ID: `PVT_kwHOAr08hs4BSRTi`

### Session End Protocol

Before ending any session:

1. **Ensure all in-progress issues have current status** — Comment on any issue where work was done this session
2. **Close completed issues** — Any issue where all acceptance criteria are met
3. **Set project board fields** — All items touched this session have correct Status, Agent, Effort, Sprint

### When Creating New Issues

Every new issue must be:
1. Created with appropriate labels (`type:`, `priority:`, `module:`)
2. Added to the project: `gh project item-add 2 --owner rmkohlman --url <issue-url>`
3. Fields set immediately: Status, Agent, Effort, Sprint (if applicable)

---

## Directory Structure

```
~/Developer/tools/MaestroVault/
├── CLAUDE.md              <- This file
├── README.md              <- Project README
├── .opencode/
│   ├── agents/            <- Agent definitions (7 agents)
│   └── commands/          <- Custom slash commands (/plan, /release)
├── cmd/mav/               <- CLI entry point (Cobra)
├── internal/
│   ├── api/               <- REST API server
│   ├── clipboard/         <- Clipboard integration
│   ├── crypto/            <- AES-256-GCM encryption
│   ├── keychain/          <- macOS Keychain master key
│   ├── store/             <- SQLite storage
│   ├── touchid/           <- TouchID authentication
│   ├── tui/               <- Bubble Tea terminal UI
│   └── vault/             <- Core vault logic + config
├── pkg/client/            <- Public Go client library
├── docs/                  <- MkDocs Material site source
├── .github/workflows/     <- CI/CD (release.yml, docs.yml)
└── .goreleaser.yaml       <- GoReleaser config
```

## GitHub Resources

| Resource | Visibility |
|----------|------------|
| `rmkohlman/MaestroVault` | Public |
| `rmkohlman/homebrew-tap` | Public |
| GitHub Project #2 "MaestroVault" | Private |

## Current State

- **Version:** v0.9.0
- **See GitHub Project for active work**
