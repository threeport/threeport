package v0

import "testing"

func TestEnvOrReturnsSetValue(t *testing.T) {
	t.Setenv("TPT_TEST_ENVOR", "  hello  ")
	if got := EnvOr("TPT_TEST_ENVOR", "fallback"); got != "hello" {
		t.Errorf("EnvOr() = %q, want %q", got, "hello")
	}
}

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

func TestEnvOrFallsBackOnEmptyOrWhitespace(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
	}{
		{"unset", false, ""},
		{"empty", true, ""},
		{"whitespace only", true, "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv("TPT_TEST_ENVOR", c.val)
			}
			if got := EnvOr("TPT_TEST_ENVOR", "fallback"); got != "fallback" {
				t.Errorf("EnvOr() = %q, want %q", got, "fallback")
			}
		})
	}
}
