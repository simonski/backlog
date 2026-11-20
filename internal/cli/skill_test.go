package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"first installation", args, fmt.Sprintf("Installed reporting skill at %s\n", path)},
		{"repeat installation", args, fmt.Sprintf("Reporting skill already installed at %s\n", path)},
		{"JSON receipt", append(args, "-json"), ""},
	} {
		var out, diagnostics bytes.Buffer
		if code := app.Run(t.Context(), tc.args, nil, &out, &diagnostics); code != 0 {
			t.Fatalf("%s exited %d: %s", tc.name, code, &diagnostics)
		}
		want := tc.want
		if want == "" {
			pathJSON, err := json.Marshal(path)
			if err != nil {
				t.Fatal(err)
			}
			want = fmt.Sprintf("{\n  \"path\": %s,\n  \"installed\": false\n}\n", pathJSON)
		}
		if out.String() != want || diagnostics.Len() != 0 {
			t.Fatalf("%s: stdout=%q stderr=%q", tc.name, &out, &diagnostics)
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
