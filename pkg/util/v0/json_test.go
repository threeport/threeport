package v0

import (
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/datatypes"
)

func TestUpdateNamespace(t *testing.T) {
	tests := []struct {
		name      string
		in        datatypes.JSON
		ns        string
		wantNS    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "sets namespace for non-Gateway kinds",
			in:     datatypes.JSON(`{"kind":"Deployment","metadata":{"name":"x"}}`),
			ns:     "default",
			wantNS: "default",
		},
		{
			name:   "Gateway kind forces GatewaySystemNamespace",
			in:     datatypes.JSON(`{"kind":"Gateway","metadata":{"name":"x","namespace":"ignored"}}`),
			ns:     "default",
			wantNS: GatewaySystemNamespace,
		},
		{
			name:      "missing metadata returns error",
			in:        datatypes.JSON(`{"kind":"Deployment"}`),
			ns:        "default",
			wantErr:   true,
			errSubstr: `failed to find "metadata"`,
		},
		{
			name:      "invalid JSON returns error",
			in:        datatypes.JSON(`{"kind":`),
			ns:        "default",
			wantErr:   true,
			errSubstr: "failed to unmarshal JSON definition to map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := UpdateNamespace(tt.in, tt.ns)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("error=%q, want substring %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("failed to unmarshal output: %v; out=%s", err, string(out))
			}
			meta, ok := got["metadata"].(map[string]any)
			if !ok {
				t.Fatalf("output missing metadata: %v", got)
			}
			if gotNS, _ := meta["namespace"].(string); gotNS != tt.wantNS {
				t.Fatalf("namespace=%q, want %q; out=%v", gotNS, tt.wantNS, got)
			}
		})
	}
}

