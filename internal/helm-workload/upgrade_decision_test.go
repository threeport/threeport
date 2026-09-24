package helmworkload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/release"
	helmtime "helm.sh/helm/v3/pkg/time"
)

// These cover the decision that stands between a reconcile and a real helm
// upgrade. Getting it wrong in one direction records a revision and re-applies
// the workload for nothing; getting it wrong in the other silently drops a
// change the chart asked for.

// hook returns a hook as a freshly rendered release carries it.
func hook(name, manifest string, events ...release.HookEvent) *release.Hook {
	return &release.Hook{
		Name:     name,
		Kind:     "Job",
		Path:     "templates/" + name + ".yaml",
		Manifest: manifest,
		Events:   events,
		Weight:   1,
	}
}

func TestReleaseMatchesRender_UnchangedRelease(t *testing.T) {
	deployed := &release.Release{Manifest: "kind: ConfigMap\n"}
	previewed := &release.Release{Manifest: "kind: ConfigMap\n"}

	assert.True(t, releaseMatchesRender(deployed, previewed))
}

func TestReleaseMatchesRender_ChangedManifest(t *testing.T) {
	deployed := &release.Release{Manifest: "kind: ConfigMap\n"}
	previewed := &release.Release{Manifest: "kind: ConfigMap\ndata: {}\n"}

	assert.False(t, releaseMatchesRender(deployed, previewed))
}

// helm keeps hooks apart from the manifest, so a chart change touching only a
// hook leaves the manifests identical. Skipping on the manifest alone would drop
// the hook without recording or running it.
func TestReleaseMatchesRender_ChangedHookOnly(t *testing.T) {
	manifest := "kind: ConfigMap\n"
	deployed := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v1", release.HookPreUpgrade)},
	}
	previewed := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v2", release.HookPreUpgrade)},
	}

	assert.False(t, releaseMatchesRender(deployed, previewed))
}

func TestReleaseMatchesRender_AddedAndRemovedHooks(t *testing.T) {
	manifest := "kind: ConfigMap\n"
	withHook := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v1", release.HookPreUpgrade)},
	}
	withoutHook := &release.Release{Manifest: manifest}

	assert.False(t, releaseMatchesRender(withHook, withoutHook))
	assert.False(t, releaseMatchesRender(withoutHook, withHook))
}

func TestReleaseMatchesRender_ChangedHookEvents(t *testing.T) {
	manifest := "kind: ConfigMap\n"
	deployed := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v1", release.HookPreUpgrade)},
	}
	previewed := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v1", release.HookPostUpgrade)},
	}

	assert.False(t, releaseMatchesRender(deployed, previewed))
}

// helm writes a hook's LastRun when it executes, so a deployed hook carries a
// timestamp the freshly rendered one never has. Comparing whole hooks would find
// a difference on every release that has ever run one, and the upgrade would
// never be skipped.
func TestReleaseMatchesRender_ExecutedHookStillMatches(t *testing.T) {
	manifest := "kind: ConfigMap\n"
	executed := hook("migrate", "image: app:v1", release.HookPreUpgrade)
	executed.LastRun = release.HookExecution{
		StartedAt:   helmtime.Now(),
		CompletedAt: helmtime.Now(),
		Phase:       release.HookPhaseSucceeded,
	}

	deployed := &release.Release{Manifest: manifest, Hooks: []*release.Hook{executed}}
	previewed := &release.Release{
		Manifest: manifest,
		Hooks:    []*release.Hook{hook("migrate", "image: app:v1", release.HookPreUpgrade)},
	}

	assert.True(t, releaseMatchesRender(deployed, previewed))
}

// a release that could not be read is not a match: the upgrade is what would
// put it right
func TestReleaseMatchesRender_MissingReleaseIsNotAMatch(t *testing.T) {
	previewed := &release.Release{Manifest: "kind: ConfigMap\n"}

	assert.False(t, releaseMatchesRender(nil, previewed))
	assert.False(t, releaseMatchesRender(previewed, nil))
}
