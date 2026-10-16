package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

func openBareRepository(path string) (*git.Repository, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	fs := osfs.New(path)
	stor := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())
	return git.Open(stor, nil)
}

func openFromEnv() (*git.Repository, string, error) {
	gitDir := os.Getenv("GIT_DIR")
	if gitDir == "" {
		return nil, "", fmt.Errorf("GIT_DIR not set")
	}
	gitDir, err := filepath.Abs(gitDir)
	if err != nil {
		return nil, "", err
	}
	dotgitFs := osfs.New(gitDir)
	stor := filesystem.NewStorage(dotgitFs, cache.NewObjectLRUDefault())

	workTree := os.Getenv("GIT_WORK_TREE")
	if workTree != "" {
		workTree, err = filepath.Abs(workTree)
		if err != nil {
			return nil, "", err
		}
		wtFs := osfs.New(workTree)
		repo, err := git.Open(stor, wtFs)
		return repo, workTree, err
	}

	repo, err := git.Open(stor, nil)
	return repo, gitDir, err
}

// openRepository opens the repository at path, which may be a working tree
// (or a directory inside one), a linked worktree, or a bare repository.
func openRepository(path string) (*git.Repository, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
		// Linked worktrees keep their objects and refs in the main
		// repository's directory.
		EnableDotGitCommonDir: true,
	})
	if err == nil {
		return repo, nil
	}
	if bare, bareErr := openBareRepository(path); bareErr == nil {
		return bare, nil
	}
	return nil, err
}

// openExclusive returns a second handle on repo for use by a single
// goroutine, with pack files kept open between object reads. Reopening a pack
// for every object roughly doubles the time to walk a large history. Kept
// pack files are not safe for concurrent use, so the handle must not be
// shared. Call the returned close function when done. Repositories not
// backed by the filesystem are returned as is.
func openExclusive(repo *git.Repository) (*git.Repository, func()) {
	s, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return repo, func() {}
	}
	stor := filesystem.NewStorageWithOptions(s.Filesystem(), cache.NewObjectLRUDefault(), filesystem.Options{KeepDescriptors: true})
	exclusive, err := git.Open(stor, nil)
	if err != nil {
		return repo, func() {}
	}
	return exclusive, func() { stor.Close() }
}

// version is set at build time with -ldflags "-X main.version=...". When
// empty, the version recorded by the Go toolchain is used.
var version string

// buildVersion returns the version to report for --version.
func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "(devel)"
	}
	return info.Main.Version
}

// cliOptions holds the parsed command line.
type cliOptions struct {
	path    string // repository path, or "" if none was given
	version bool
}

// parseArgs parses the command line, without the program name. Usage and
// errors are written to out.
func parseArgs(args []string, out io.Writer) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("gittop", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.BoolVar(&opts.version, "version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintf(out, "Usage: gittop [flags] [path]\n\n")
		fmt.Fprintf(out, "Visualize the Git repository at path, or the current directory.\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	switch fs.NArg() {
	case 0:
	case 1:
		opts.path = fs.Arg(0)
	default:
		err := fmt.Errorf("too many arguments: %q (flags go before the path)", fs.Args()[1:])
		fmt.Fprintf(out, "%v\n", err)
		fs.Usage()
		return opts, err
	}
	return opts, nil
}

func main() {
	cli, err := parseArgs(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		os.Exit(2)
	}
	if cli.version {
		fmt.Printf("gittop %s\n", buildVersion())
		return
	}

	path := cli.path
	if path == "" {
		path = "."
	}

	var repo *git.Repository
	if cli.path == "" {
		if r, p, err := openFromEnv(); err == nil {
			repo, path = r, p
		}
	}
	if repo == nil {
		repo, err = openRepository(path)
		if errors.Is(err, git.ErrRepositoryNotExists) {
			fmt.Fprintf(os.Stderr, "Error: not a git repository: %s\n", path)
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot open %s: %v\n", path, err)
			os.Exit(1)
		}
	}

	m := newModel(repo, path)
	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if m.mouse {
		opts = append(opts, tea.WithMouseCellMotion())
	}
	p := tea.NewProgram(m, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
