package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- create / delete / list --------------------------------------------

func TestCreateDefaultsToHomeGroves(t *testing.T) {
	cfgFile := testEnv(t)
	home, _ := os.UserHomeDir()

	out, err := runCLI(t, "--config", cfgFile, "create", "demo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := filepath.Join(home, "groves", "demo")
	if !strings.Contains(out, want) {
		t.Errorf("output = %q, want it to name %s", out, want)
	}
	if got := loadConfigFile(t, cfgFile).Groves["demo"].Path; got != want {
		t.Errorf("grove path = %q, want %q", got, want)
	}
}

func TestCreateRejectsDuplicates(t *testing.T) {
	cfgFile := testEnv(t)
	runCLI(t, "--config", cfgFile, "create", "demo")
	if _, err := runCLI(t, "--config", cfgFile, "create", "demo"); err == nil {
		t.Fatal("create succeeded for a name already in use")
	}
}

func TestListShowsRepoAndDirCounts(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":main", docs)

	out, err := runCLI(t, "--config", cfgFile, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "GROVE") || !strings.Contains(out, "DIRS") {
		t.Errorf("missing table header:\n%s", out)
	}
	fields := strings.Fields(lineContaining(t, out, "demo"))
	// GROVE PATH REPOS DIRS
	if len(fields) != 4 || fields[2] != "1" || fields[3] != "1" {
		t.Errorf("row = %v, want one repo and one dir", fields)
	}
}

func TestListWithNoGroves(t *testing.T) {
	cfgFile := testEnv(t)
	out, err := runCLI(t, "--config", cfgFile, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no groves configured") {
		t.Errorf("output = %q", out)
	}
}

func TestDeleteRemovesFromConfigOnly(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", docs)
	runCLI(t, "--config", cfgFile, "sync", "demo")

	if _, err := runCLI(t, "--config", cfgFile, "delete", "demo"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := loadConfigFile(t, cfgFile).Groves["demo"]; ok {
		t.Error("grove still in the config")
	}
	if _, err := os.Stat(filepath.Join(grovePath, "docs")); err != nil {
		t.Errorf("delete without --prune removed files from disk: %v", err)
	}
}

func TestDeletePruneClearsTheGrove(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	examples := makeDir(t, filepath.Join(tmp, "examples"), "e.txt")

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)
	runCLI(t, "--config", cfgFile, "add", "demo", examples, "--symlink")
	runCLI(t, "--config", cfgFile, "sync", "demo")

	if _, err := runCLI(t, "--config", cfgFile, "delete", "demo", "--prune"); err != nil {
		t.Fatalf("delete --prune: %v", err)
	}
	for _, name := range []string{"backend", "docs", "examples"} {
		if _, err := os.Lstat(filepath.Join(grovePath, name)); !os.IsNotExist(err) {
			t.Errorf("%s survived the prune: %v", name, err)
		}
	}
	// Every source is left exactly as it was.
	for _, src := range []string{
		filepath.Join(repo, "file.txt"),
		filepath.Join(docs, "README.md"),
		filepath.Join(examples, "e.txt"),
	} {
		if _, err := os.Stat(src); err != nil {
			t.Errorf("source %s was damaged: %v", src, err)
		}
	}
}

func TestDeleteUnknownGrove(t *testing.T) {
	cfgFile := testEnv(t)
	if _, err := runCLI(t, "--config", cfgFile, "delete", "nope"); err == nil {
		t.Fatal("delete succeeded for a grove that does not exist")
	}
}

// --- sync --------------------------------------------------------------

func TestSyncMaterializesEverything(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	examples := makeDir(t, filepath.Join(tmp, "examples"), "e.txt")

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)
	runCLI(t, "--config", cfgFile, "add", "demo", examples, "--symlink")

	out, err := runCLI(t, "--config", cfgFile, "sync", "demo")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, want := range []string{"adding worktree", "copied", "symlinked"} {
		if !strings.Contains(out, want) {
			t.Errorf("sync output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(grovePath, "docs", "README.md")); err != nil {
		t.Errorf("directory not copied: %v", err)
	}
	fi, err := os.Lstat(filepath.Join(grovePath, "examples"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("--symlink entry was copied instead of linked")
	}
}

func TestSyncAllGroves(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	one := makeDir(t, filepath.Join(tmp, "one"), "a.txt")
	two := makeDir(t, filepath.Join(tmp, "two"), "b.txt")

	runCLI(t, "--config", cfgFile, "create", "alpha", "--path", filepath.Join(tmp, "g-alpha"))
	runCLI(t, "--config", cfgFile, "create", "beta", "--path", filepath.Join(tmp, "g-beta"))
	runCLI(t, "--config", cfgFile, "add", "alpha", one)
	runCLI(t, "--config", cfgFile, "add", "beta", two)

	if _, err := runCLI(t, "--config", cfgFile, "sync"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, p := range []string{
		filepath.Join(tmp, "g-alpha", "one", "a.txt"),
		filepath.Join(tmp, "g-beta", "two", "b.txt"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestSyncUnknownGrove(t *testing.T) {
	cfgFile := testEnv(t)
	if _, err := runCLI(t, "--config", cfgFile, "sync", "nope"); err == nil {
		t.Fatal("sync succeeded for a grove that does not exist")
	}
}

// --- status ------------------------------------------------------------

// Groves with no plain directories keep the original single-table output, so
// anything parsing it keeps working.
func TestStatusOmitsTheDirTableWhenThereAreNone(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x")

	out, err := runCLI(t, "--config", cfgFile, "status", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CONFIG BRANCH") {
		t.Errorf("repo table missing:\n%s", out)
	}
	if strings.Contains(out, "MODE") {
		t.Errorf("directory table printed for a grove with no directories:\n%s", out)
	}
}

func TestStatusReportsBothTables(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"), "README.md")
	later := makeDir(t, filepath.Join(tmp, "later"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":feat/x", docs)
	runCLI(t, "--config", cfgFile, "sync", "demo")
	// Added after the sync, so it is still missing.
	runCLI(t, "--config", cfgFile, "add", "demo", later)

	out, err := runCLI(t, "--config", cfgFile, "status", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "DIR") || !strings.Contains(out, "MODE") {
		t.Errorf("directory table missing:\n%s", out)
	}
	if got := lineContaining(t, out, "backend"); !strings.Contains(got, "OK") {
		t.Errorf("repo row = %q, want OK", got)
	}
	if got := lineContaining(t, out, " docs "); !strings.Contains(got, "OK") || !strings.Contains(got, "copy") {
		t.Errorf("dir row = %q, want an OK copy", got)
	}
	if got := lineContaining(t, out, " later "); !strings.Contains(got, "MISSING") {
		t.Errorf("unsynced dir row = %q, want MISSING", got)
	}
	if !strings.Contains(out, "not synced — run: gitgrove sync demo") {
		t.Errorf("missing the sync hint:\n%s", out)
	}
}

func TestStatusReportsDriftAndConflict(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	grovePath := filepath.Join(tmp, "grove")
	docs := makeDir(t, filepath.Join(tmp, "docs"))
	other := makeDir(t, filepath.Join(tmp, "other"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", grovePath)
	runCLI(t, "--config", cfgFile, "add", "demo", docs, "--symlink")
	runCLI(t, "--config", cfgFile, "sync", "demo")

	// Point the symlink somewhere else behind gitgrove's back.
	link := filepath.Join(grovePath, "docs")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, link); err != nil {
		t.Fatal(err)
	}
	out, _ := runCLI(t, "--config", cfgFile, "status", "demo")
	if got := lineContaining(t, out, " docs "); !strings.Contains(got, "DRIFTED") {
		t.Errorf("row = %q, want DRIFTED", got)
	}

	// Replace it with a real directory: that is a conflict, not drift.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(link, 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ = runCLI(t, "--config", cfgFile, "status", "demo")
	if got := lineContaining(t, out, " docs "); !strings.Contains(got, "CONFLICT") {
		t.Errorf("row = %q, want CONFLICT", got)
	}
}

func TestStatusWithNoGroves(t *testing.T) {
	cfgFile := testEnv(t)
	out, err := runCLI(t, "--config", cfgFile, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no groves configured") {
		t.Errorf("output = %q", out)
	}
}

func TestStatusUnknownGrove(t *testing.T) {
	cfgFile := testEnv(t)
	runCLI(t, "--config", cfgFile, "create", "demo")
	if _, err := runCLI(t, "--config", cfgFile, "status", "nope"); err == nil {
		t.Fatal("status succeeded for a grove that does not exist")
	}
}

// --- doctor ------------------------------------------------------------

func TestDoctorPassesOnAHealthyGrove(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	repo := initRepo(t, filepath.Join(tmp, "backend"))
	docs := makeDir(t, filepath.Join(tmp, "docs"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	runCLI(t, "--config", cfgFile, "add", "demo", repo+":main", docs)

	out, err := runCLI(t, "--config", cfgFile, "doctor", "demo")
	if err != nil {
		t.Fatalf("doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, "ERROR") {
		t.Errorf("unexpected error row:\n%s", out)
	}
}

// doctor exits non-zero on errors so it is usable in scripts and CI.
func TestDoctorFailsOnAMissingDirectory(t *testing.T) {
	cfgFile := testEnv(t)
	tmp := t.TempDir()
	docs := makeDir(t, filepath.Join(tmp, "docs"))

	runCLI(t, "--config", cfgFile, "create", "demo", "--path", filepath.Join(tmp, "grove"))
	runCLI(t, "--config", cfgFile, "add", "demo", docs)
	if err := os.RemoveAll(docs); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "--config", cfgFile, "doctor", "demo")
	if err == nil {
		t.Fatalf("doctor exited zero despite a missing source:\n%s", out)
	}
	if !strings.Contains(out, "ERROR") || !strings.Contains(out, "does not exist") {
		t.Errorf("output did not explain the problem:\n%s", out)
	}
}

func TestDoctorWithNoGroves(t *testing.T) {
	cfgFile := testEnv(t)
	out, err := runCLI(t, "--config", cfgFile, "doctor")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no groves configured") {
		t.Errorf("output = %q", out)
	}
}

func TestDoctorUnknownGrove(t *testing.T) {
	cfgFile := testEnv(t)
	if _, err := runCLI(t, "--config", cfgFile, "doctor", "nope"); err == nil {
		t.Fatal("doctor succeeded for a grove that does not exist")
	}
}

// --- helpers -----------------------------------------------------------

// lineContaining returns the single output line holding substr.
func lineContaining(t *testing.T, out, substr string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("no line containing %q in:\n%s", substr, out)
	return ""
}
