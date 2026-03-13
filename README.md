# gittop

A beautiful terminal UI for visualizing Git repository statistics, inspired by htop/btop.

![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)

## Install

```
go install github.com/hjr265/gittop@latest
```

Or build from source:

```
git clone https://github.com/hjr265/gittop.git
cd gittop
go build -o gittop .
```

## Usage

```
gittop              # visualize the current directory's Git repo
gittop /path/to/repo
```

## Tabs

| # | Tab | What it shows |
|---|-----|---------------|
| 1 | **Summary** | KPI cards (total commits, active days, peak day, time span, latest release) + commit bar chart |
| 2 | **Activity** | Commit heatmap (GitHub-style contribution grid) |
| 3 | **Contributors** | Leaderboard, cadence, timeline, and file ownership views |
| 4 | **Branches** | Sortable table with last commit, author, ahead/behind counts |
| 5 | **Health** | Largest files, most churn, most authors, stalest files |
| 6 | **Releases** | Tag timeline and release cadence chart |
| 7 | **Commits** | Scrollable commit log |

## Keys

| Key | Action |
|-----|--------|
| `1`–`7` | Switch tab |
| `Tab` / `Shift+Tab` | Next / previous tab |
| `+` / `-` | Widen / narrow date range (3m, 6m, 1y, 2y, 5y, all) |
| `/` | Open filter (`author:"name"`, `path:*.go`, `"keyword"`, `and`/`or`) |
| `Esc` | Clear filter |
| `v` | Cycle sub-views (Activity, Contributors, Health, Releases) |
| `s` / `S` | Cycle sort column / toggle order (Branches) |
| `j` / `k` | Scroll down / up |
| `g` / `G` | Jump to top / bottom |
| `q` | Quit |

## Recommended Setup

For best results with the block character bar charts:

- **Terminal:** Kitty, Alacritty, or WezTerm
- **Font:** JetBrains Mono, Iosevka, or Fira Code

Prototyped rapidly with an agentic coding tool.

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.
