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
			name:         "ghcr registry",
			image:        "ghcr.io/threeport/threeport-agent:v0.7.0",
			wantRegistry: "ghcr.io/threeport",
			wantName:     "threeport-agent",
			wantTag:      "v0.7.0",
		},
		{
			name:         "artifact registry",
			image:        "us-central1-docker.pkg.dev/example-project/threeport/threeport-agent:tag",
			wantRegistry: "us-central1-docker.pkg.dev/example-project/threeport",
			wantName:     "threeport-agent",
			wantTag:      "tag",
		},
		{
			name:         "dev registry with a port",
			image:        "localhost:5001/threeport-agent:dev",
			wantRegistry: "localhost:5001",
			wantName:     "threeport-agent",
			wantTag:      "dev",
		},
		{
			name:    "no registry",
			image:   "myimage:tag",
			wantErr: true,
		},
		{
			name:    "no tag",
			image:   "gcr.io/myimage",
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
