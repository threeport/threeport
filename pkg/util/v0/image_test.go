package v0

import "testing"

func TestParseImage(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		wantRegistry string
		wantName     string
		wantTag      string
		wantErr      bool
	}{
		{
			name:         "single-segment registry",
			image:        "gcr.io/myimage:1.2.3",
			wantRegistry: "gcr.io",
			wantName:     "myimage",
			wantTag:      "1.2.3",
		},
		{
			name:         "two-segment registry",
			image:        "docker.io/library/nginx:latest",
			wantRegistry: "docker.io/library",
			wantName:     "nginx",
			wantTag:      "latest",
		},
		{
			name:    "invalid format",
			image:   "example.com/a/b/c:tag",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRegistry, gotName, gotTag, err := ParseImage(tt.image)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseImage(%q) expected error, got nil", tt.image)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseImage(%q) unexpected error: %v", tt.image, err)
			}
			if gotRegistry != tt.wantRegistry || gotName != tt.wantName || gotTag != tt.wantTag {
				t.Fatalf("ParseImage(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tt.image, gotRegistry, gotName, gotTag, tt.wantRegistry, tt.wantName, tt.wantTag)
			}
		})
	}
}

// This documents the current behavior when no tag is present: ParseImage panics.
func TestParseImage_MissingTagPanics(t *testing.T) {
	image := "gcr.io/myimage"

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("ParseImage(%q) expected panic due to missing tag, but did not panic", image)
		}
	}()

	ParseImage(image)
}

