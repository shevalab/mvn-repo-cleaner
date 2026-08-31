package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the parsed, validated command-line configuration.
type Config struct {
	Paths    []string
	Confirm  bool
	Repo     string
	Export   string
	FromFile string
	// Mode is one of "scan", "from-file", "export".
	Mode string
}

// Parse reads and validates command-line arguments.
func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("mvn-repo-cleaner", flag.ContinueOnError)
	var (
		confirm  = fs.Bool("confirm", false, "actually delete stale dependencies")
		repo     = fs.String("repo", "", "local maven repository root (default ~/.m2/repository)")
		export   = fs.String("export", "", "write the stale list to <file> (export-only; no deletion)")
		fromFile = fs.String("from-file", "", "delete exactly the paths listed in <file> (no scan)")
	)
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: mvn-repo-cleaner [options] [project-paths...]\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &Config{
		Paths:    fs.Args(),
		Confirm:  *confirm,
		Repo:     *repo,
		Export:   *export,
		FromFile: *fromFile,
	}

	if cfg.Repo == "" {
		cfg.Repo = defaultRepo()
	}
	abs, err := filepath.Abs(cfg.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve repo path: %w", err)
	}
	cfg.Repo = abs

	switch {
	case cfg.Export != "":
		if cfg.Confirm || cfg.FromFile != "" {
			return nil, fmt.Errorf("--export is export-only and cannot be combined with --confirm or --from-file")
		}
		cfg.Mode = "export"
	case cfg.FromFile != "":
		cfg.Mode = "from-file"
	default:
		cfg.Mode = "scan"
	}

	if cfg.Mode == "scan" && len(cfg.Paths) == 0 {
		return nil, fmt.Errorf("scan mode requires at least one project path")
	}

	return cfg, nil
}

func defaultRepo() string {
	if h := os.Getenv("HOME"); h != "" {
		return filepath.Join(h, ".m2", "repository")
	}
	return ".m2/repository"
}
