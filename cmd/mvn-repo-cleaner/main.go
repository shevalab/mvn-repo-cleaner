package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shevalab/mvn-repo-cleaner/internal/cli"
	"github.com/shevalab/mvn-repo-cleaner/internal/model"
	"github.com/shevalab/mvn-repo-cleaner/internal/repo"
	"github.com/shevalab/mvn-repo-cleaner/internal/resolve"
	"github.com/shevalab/mvn-repo-cleaner/internal/scan"
)

func main() {
	cfg, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	switch cfg.Mode {
	case "export":
		runExport(cfg)
	case "from-file":
		runFromFile(cfg)
	default:
		runScan(cfg)
	}
}

func runExport(cfg *cli.Config) {
	stale := computeStale(cfg)
	if err := repo.WriteList(cfg.Export, stale); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("exported %d stale paths to %s\n", len(stale), cfg.Export)
}

func runFromFile(cfg *cli.Config) {
	paths, err := repo.ReadList(cfg.FromFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if !cfg.Confirm {
		fmt.Printf("dry-run: would delete %d path(s) from %s\n", len(paths), cfg.FromFile)
		for _, p := range paths {
			fmt.Println(p)
		}
		return
	}
	removed, failed := repo.DeleteVersions(paths)
	fmt.Printf("deleted %d path(s)", len(removed))
	if len(failed) > 0 {
		fmt.Printf("; %d failed", len(failed))
	}
	fmt.Println()
	for _, p := range removed {
		fmt.Println("deleted:", p)
	}
	for _, p := range failed {
		fmt.Fprintln(os.Stderr, "failed:", p)
	}
}

func runScan(cfg *cli.Config) {
	stale := computeStale(cfg)
	if cfg.Confirm {
		removed, failed := repo.DeleteVersions(stale)
		fmt.Printf("deleted %d path(s)", len(removed))
		if len(failed) > 0 {
			fmt.Printf("; %d failed", len(failed))
		}
		fmt.Println()
		for _, p := range removed {
			fmt.Println("deleted:", p)
		}
		for _, p := range failed {
			fmt.Fprintln(os.Stderr, "failed:", p)
		}
	} else {
		fmt.Printf("dry-run: %d stale path(s) would be deleted\n", len(stale))
		for _, p := range stale {
			fmt.Println(p)
		}
	}
}

// computeStale resolves in-use set and diffs against the on-disk repo.
func computeStale(cfg *cli.Config) []string {
	loader := &resolve.Loader{Repo: cfg.Repo}
	useSet := scan.NewInUseSet()
	for _, path := range cfg.Paths {
		fmt.Printf("Processing %s\n", path)
		poms, err := scan.FindPoms(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "warn: scan", path, err)
			continue
		}
		for _, pom := range poms {
			// Print the folder being processed relative to the project root.
			if rel, err := filepath.Rel(path, pom); err == nil {
				fmt.Printf("%s\n", rel)
			} else {
				fmt.Printf("%s\n", pom)
			}
			res, err := scan.ScanProject(loader, pom)
			if err != nil {
				fmt.Fprintln(os.Stderr, "warn: resolve", pom, err)
				continue
			}
			useSet.Add(res)
		}
	}

	onDisk, err := repo.Walk(cfg.Repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: walk repo:", err)
		os.Exit(1)
	}

	var stale []string
	for art, versions := range onDisk {
		inUseVersions, present := useSet[art]
		// present=false: artifact not used at all -> every version stale.
		// present=true but empty map or keep-all sentinel: in-use at unknown
		// version -> keep all.
		if !present {
			for _, v := range versions {
				stale = append(stale, repo.VersionDir(cfg.Repo, art.GroupID, art.ArtifactID, v))
			}
			continue
		}
		if len(inUseVersions) == 0 || inUseVersions[model.KeepAllVersion] {
			// The artifact is in use but no concrete version could be
			// resolved (e.g. unresolvable parent/management). Be conservative
			// and keep every version.
			continue
		}
		for _, v := range versions {
			if !inUseVersions[v] {
				stale = append(stale, repo.VersionDir(cfg.Repo, art.GroupID, art.ArtifactID, v))
			}
		}
	}
	// Sanitize paths to be absolute & clean.
	for i, p := range stale {
		if a, err := filepath.Abs(p); err == nil {
			stale[i] = a
		}
	}
	return stale
}
