package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallSkillPreservesExistingCopy(t *testing.T) {
	contents := []byte("# Embedded skill\n")
	app := App{ReportingSkill: contents}
	directory := t.TempDir()
	path := filepath.Join(directory, "report-harness-progress", "SKILL.md")
	args := []string{"install-skill", "-dir", directory}

	for _, wantInstalled := range []bool{true, false} {
		var out, diagnostics bytes.Buffer
		if code := app.Run(t.Context(), args, nil, &out, &diagnostics); code != 0 {
			t.Fatalf("install-skill exited %d: %s", code, &diagnostics)
		}
		var receipt struct {
			Path      string `json:"path"`
			Installed bool   `json:"installed"`
		}
		if err := json.Unmarshal(out.Bytes(), &receipt); err != nil {
			t.Fatal(err)
		}
		if receipt.Path != path || receipt.Installed != wantInstalled {
			t.Fatalf("unexpected receipt: %+v", receipt)
		}
	}

	customized := []byte("# Customized skill\n")
	if err := os.WriteFile(path, customized, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, diagnostics bytes.Buffer
	if code := app.Run(t.Context(), args, nil, &out, &diagnostics); code == 0 {
		t.Fatal("installation over a customized skill succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, customized) {
		t.Fatalf("customized skill changed: %q", got)
	}
}
