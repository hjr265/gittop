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

Press `q`, `Esc`, or `Ctrl+C` to quit.

## Recommended Setup

For best results with the block character bar charts:

- **Terminal:** Kitty, Alacritty, or WezTerm
- **Font:** JetBrains Mono, Iosevka, or Fira Code

## License

BSD 3-Clause License. See [LICENSE](LICENSE) for details.
