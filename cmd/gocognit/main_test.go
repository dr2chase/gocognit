package main

import (
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/uudashr/gocognit"
)

func testStat(filename string, line int, name string) gocognit.Stat {
	return gocognit.Stat{
		FuncName: name,
		Pos:      token.Position{Filename: filename, Line: line, Column: 1},
	}
}

func TestDedupeStats(t *testing.T) {
	in := []gocognit.Stat{
		testStat("/p/a.go", 1, "A"),
		testStat("/p/a.go", 1, "A"), // duplicate from a test variant
		testStat("/p/a.go", 2, "B"),
		testStat("/p/b.go", 1, "A"), // same name, different position
	}

	got := dedupeStats(in)
	if len(got) != 3 {
		t.Fatalf("dedupeStats returned %d stats, want 3: %v", len(got), got)
	}
}

func TestStatFileWithinTarget(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		target   string
		isFile   bool
		want     bool
	}{
		{name: "dir direct child", filename: "/p/a.go", target: "/p", want: true},
		{name: "dir nested child", filename: "/p/sub/a.go", target: "/p", want: true},
		{name: "dir outside", filename: "/q/a.go", target: "/p", want: false},
		{name: "dir prefix is not membership", filename: "/px/a.go", target: "/p", want: false},
		{name: "file exact", filename: "/p/a.go", target: "/p/a.go", isFile: true, want: true},
		{name: "file other", filename: "/p/b.go", target: "/p/a.go", isFile: true, want: false},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := statFileWithinTarget(tt.filename, tt.target, tt.isFile); got != tt.want {
				t.Errorf("statFileWithinTarget(%q, %q, %v) = %v, want %v", tt.filename, tt.target, tt.isFile, got, tt.want)
			}
		})
	}
}

func TestFilterStatFiles(t *testing.T) {
	stats := []gocognit.Stat{
		testStat("/p/a.go", 1, "A"),
		testStat("/p/sub/b.go", 1, "B"),
		testStat("/other/c.go", 1, "C"),
	}

	dir := filterStatFiles(stats, "/p", false)
	if len(dir) != 2 {
		t.Fatalf("dir filter kept %d stats, want 2: %v", len(dir), dir)
	}

	file := filterStatFiles(stats, "/p/a.go", true)
	if len(file) != 1 || file[0].FuncName != "A" {
		t.Fatalf("file filter = %v, want only A", file)
	}
}

func TestIgnoreUnparseableFilesFullPath(t *testing.T) {
	tmp := t.TempDir()

	validGo := []byte("package p\n\nfunc Ok(x bool) {\n\tif x {\n\t}\n}\n")
	if err := os.WriteFile(filepath.Join(tmp, "ok.go"), validGo, 0o600); err != nil {
		t.Fatal(err)
	}

	testdataDir := filepath.Join(tmp, "testdata")
	if err := os.MkdirAll(testdataDir, 0o755); err != nil {
		t.Fatal(err)
	}

	badFile := filepath.Join(testdataDir, "broken.go")
	if err := os.WriteFile(badFile, []byte("package p\nfunc broken( {\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Without -ignore, analyzing the directory fails on the unparseable file.
	if _, err := analyze([]string{tmp}, true, nil, false, false); err == nil {
		t.Fatal("expected parse error without ignore regexp, got nil")
	}

	// With -ignore matching "/testdata/" against the full path, analyzing "."
	// from inside tmp or inside tmp/testdata skips broken.go before parsing.
	re := regexp.MustCompile(`/testdata/`)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	stats, err := analyze([]string{"."}, true, re, false, false)
	if err != nil {
		t.Fatalf("analyze(\".\") from parent dir failed: %v", err)
	}
	if len(stats) != 1 || stats[0].FuncName != "Ok" {
		t.Fatalf("got stats %v, want [Ok]", stats)
	}

	if err := os.Chdir(testdataDir); err != nil {
		t.Fatal(err)
	}

	stats, err = analyze([]string{"."}, true, re, false, false)
	if err != nil {
		t.Fatalf("analyze(\".\") from testdata dir failed: %v", err)
	}
	if len(stats) != 0 {
		t.Fatalf("got stats %v, want empty", stats)
	}

	stats, err = analyze([]string{"broken.go"}, true, re, false, false)
	if err != nil {
		t.Fatalf("analyze(\"broken.go\") from testdata dir failed: %v", err)
	}
	if len(stats) != 0 {
		t.Fatalf("got stats %v, want empty", stats)
	}
}
