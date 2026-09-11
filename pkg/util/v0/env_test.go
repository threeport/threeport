package v0

import "testing"

func TestImageBuildParallelismParsesValidCount(t *testing.T) {
	t.Setenv("PARALLEL_IMAGE_BUILD", "4")
	if got := ImageBuildParallelism(); got != 4 {
		t.Errorf("ImageBuildParallelism() = %d, want 4", got)
	}
}

func TestImageBuildParallelismFloorsInvalidAndNonPositive(t *testing.T) {
	for _, in := range []string{"abc", "0", "-3"} {
		t.Run(in, func(t *testing.T) {
			t.Setenv("PARALLEL_IMAGE_BUILD", in)
			if got := ImageBuildParallelism(); got != 1 {
				t.Errorf("ImageBuildParallelism(%q) = %d, want 1", in, got)
			}
		})
	}
}
