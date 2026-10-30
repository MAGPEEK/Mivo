package archtest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryFollowsLayerMatrix(t *testing.T) {
	t.Parallel()

	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}

	moduleRoot, err := findModuleRoot(workingDir)
	if err != nil {
		t.Fatalf("find module root: %v", err)
	}

	report, err := checkTree(moduleRoot, layerMatrix())
	if err != nil {
		t.Fatalf("check tree: %v", err)
	}

	// Guards against a silently empty walk (wrong root) passing as "no violations".
	if report.checkedFiles == 0 {
		t.Fatalf("no Go files found below %s", filepath.Join(moduleRoot, internalDir))
	}

	for _, violation := range report.violations {
		t.Errorf("layer violation: %s", violation)
	}
}

func TestCheckTreeFindsViolationsAndSkipsTestsAndTestdata(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "go.mod", "module "+modulePath+"\n")
	writeFile(t, root, "internal/domain/media/item.go", "package media\nimport \"os\"\n")
	writeFile(t, root, "internal/domain/media/item_test.go", "package media\nimport \"os\"\n")
	writeFile(t, root, "internal/domain/media/testdata/fixture.go", "package fixture\nimport \"os\"\n")
	writeFile(t, root, "internal/domain/media/notes.txt", "import \"os\"\n")

	report, err := checkTree(root, layerMatrix())
	if err != nil {
		t.Fatalf("check tree: %v", err)
	}

	if report.checkedFiles != 1 || len(report.violations) != 1 {
		t.Fatalf("checked %d files with violations %v, want 1 file with 1 violation",
			report.checkedFiles, report.violations)
	}

	if got := report.violations[0].File; got != "internal/domain/media/item.go" {
		t.Errorf("violation file = %q, want internal/domain/media/item.go", got)
	}
}

func TestCheckTreeFailsWithoutInternalDirectory(t *testing.T) {
	t.Parallel()

	if _, err := checkTree(t.TempDir(), layerMatrix()); err == nil {
		t.Fatal("checkTree succeeded without an internal directory")
	}
}

func TestFindModuleRootWalksUpToGoMod(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, root, "go.mod", "module "+modulePath+"\n")
	nested := filepath.Join(root, "internal", "archtest")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatalf("create %s: %v", nested, err)
	}

	got, err := findModuleRoot(nested)
	if err != nil {
		t.Fatalf("findModuleRoot: %v", err)
	}

	if got != root {
		t.Errorf("findModuleRoot = %q, want %q", got, root)
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
