package cli

import (
	"bytes"
	"fmt"
	"testing"

	"harness.engineering/internal/version"
)

func TestVersionOutputFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"text", []string{"version"}, fmt.Sprintf("CLI version:      %s\nServer version:   %s\nProtocol version: %s\n", version.Client, version.Server, version.Protocol)},
		{"JSON", []string{"version", "-json"}, fmt.Sprintf("{\n  \"client_version\": %q,\n  \"server_version\": %q,\n  \"protocol_version\": %d,\n  \"protocol_semver\": %q\n}\n", version.Client, version.Server, version.LegacyProtocol, version.Protocol)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			code := (App{}).Run(t.Context(), tc.args, nil, &out, &diagnostics)
			if code != 0 || diagnostics.Len() != 0 || out.String() != tc.want {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, &out, &diagnostics)
			}
		})
	}
}
