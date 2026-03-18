package v0

import "testing"

func TestParseStructTag(t *testing.T) {
	tests := []struct {
		name      string
		tagString string
		want      map[string]string
	}{
		{
			name:      "multiple keys with backticks",
			tagString: "`json:\"name,omitempty\" yaml:\"name\"`",
			want: map[string]string{
				"json": "name,omitempty",
				"yaml": "name",
			},
		},
		{
			name:      "multiple keys without backticks",
			tagString: `json:"id" validate:"required"`,
			want: map[string]string{
				"json":     "id",
				"validate": "required",
			},
		},
		{
			name:      "malformed pair still yields key with empty value",
			tagString: "`json:\"a\" badpair yaml:\"b\"`",
			want: map[string]string{
				"json":     "a",
				"badpair":  "",
				// reflect.StructTag treats the full tag as invalid once malformed,
				// so subsequent keys are present but return empty values.
				"yaml": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStructTag(tt.tagString)

			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Fatalf("key %q = (%q, present=%v), want (%q, present=true); full map=%v",
						k, gotV, ok, wantV, got)
				}
			}
		})
	}
}

