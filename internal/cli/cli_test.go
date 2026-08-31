package cli

import "testing"

func TestParseScan(t *testing.T) {
	cfg, err := Parse([]string{"/some/project"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "scan" || len(cfg.Paths) != 1 || cfg.Paths[0] != "/some/project" {
		t.Fatalf("got %+v", cfg)
	}
	if cfg.Repo == "" {
		t.Fatal("repo should default to a path")
	}
}

func TestParseScanNoPaths(t *testing.T) {
	if _, err := Parse([]string{}); err == nil {
		t.Fatal("scan mode with no paths should error")
	}
}

func TestParseExport(t *testing.T) {
	cfg, err := Parse([]string{"--export", "/tmp/list.txt", "/proj"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "export" || cfg.Export != "/tmp/list.txt" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestParseExportConflict(t *testing.T) {
	if _, err := Parse([]string{"--export", "/tmp/l.txt", "--confirm", "/proj"}); err == nil {
		t.Fatal("export with confirm should error")
	}
	if _, err := Parse([]string{"--export", "/tmp/l.txt", "--from-file", "/tmp/x.txt"}); err == nil {
		t.Fatal("export with from-file should error")
	}
}

func TestParseFromFile(t *testing.T) {
	cfg, err := Parse([]string{"--from-file", "/tmp/l.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != "from-file" || cfg.FromFile != "/tmp/l.txt" {
		t.Fatalf("got %+v", cfg)
	}
}
