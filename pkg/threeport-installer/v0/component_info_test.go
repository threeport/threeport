package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// componentByName finds a component the installer will install.
func componentByName(t *testing.T, cpi *ControlPlaneInstaller, name string) *v0.ControlPlaneComponent {
	t.Helper()
	component, ok := cpi.defaultComponents()[name]
	require.True(t, ok, "installer has no component %q", name)

	return component
}

// recorded is how a control plane records a component it runs.
func recorded(name, namespace, tag string) *v0.ControlPlaneComponent {
	return &v0.ControlPlaneComponent{Name: name, ImageNamespace: namespace, ImageTag: tag}
}

func TestDefaultComponentNames(t *testing.T) {
	names := DefaultComponentNames()

	assert.Len(t, names, len(ThreeportControllerList)+3)
	for _, name := range []string{
		"rest-api", "agent", "database-migrator", "gateway-controller", "gcp-controller",
		"machine-runtime-controller", "machine-workload-controller",
	} {
		assert.True(t, IsDefaultComponent(name), name)
	}
	assert.False(t, IsDefaultComponent("gateway-controler"))
	assert.False(t, IsDefaultComponent(""))
}

func TestValidateComponentOverrides(t *testing.T) {
	tests := []struct {
		name      string
		overrides []*v0.ControlPlaneComponent
		wantErr   string
	}{
		{name: "none"},
		{
			name:      "override of a default component",
			overrides: []*v0.ControlPlaneComponent{{Name: "gateway-controller", Enabled: util.Ptr(false)}},
		},
		{
			name:      "override of the rest api with only a tag",
			overrides: []*v0.ControlPlaneComponent{{Name: "rest-api", ImageTag: "v0.2-custom"}},
		},
		{
			name: "additional component with a full image",
			overrides: []*v0.ControlPlaneComponent{{
				Name: "my-controller", Additional: util.Ptr(true),
				ImageName: "my-controller", ImageNamespace: "ghcr.io/me", ImageTag: "v1",
			}},
		},
		{
			name:      "mistyped name supplying only a tag",
			overrides: []*v0.ControlPlaneComponent{{Name: "gateway-controler", ImageTag: "v1"}},
			wantErr:   `"gateway-controler" is not a default component`,
		},
		{
			name:      "unknown name that is not marked additional",
			overrides: []*v0.ControlPlaneComponent{{Name: "my-controller", ImageName: "x", ImageNamespace: "y", ImageTag: "z"}},
			wantErr:   `set Additional to add a component of your own`,
		},
		{
			name:      "additional component without an image",
			overrides: []*v0.ControlPlaneComponent{{Name: "my-controller", Additional: util.Ptr(true), ImageTag: "v1"}},
			wantErr:   "must set ImageName, ImageNamespace and ImageTag",
		},
		{
			name:      "default component marked additional",
			overrides: []*v0.ControlPlaneComponent{{Name: "rest-api", Additional: util.Ptr(true)}},
			wantErr:   "cannot be marked Additional",
		},
		{
			name: "same component twice",
			overrides: []*v0.ControlPlaneComponent{
				{Name: "agent", ImageTag: "v1"},
				{Name: "agent", ImageTag: "v2"},
			},
			wantErr: `"agent" is given more than once`,
		},
		{
			name:      "no name",
			overrides: []*v0.ControlPlaneComponent{{ImageTag: "v1"}},
			wantErr:   "has no name",
		},
		{
			name:      "empty entry",
			overrides: []*v0.ControlPlaneComponent{nil},
			wantErr:   "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComponentOverrides(tt.overrides)
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestConfigureChildComponents_NoParentRecordKeepsDefaults(t *testing.T) {
	cpi := NewInstaller()

	require.NoError(t, cpi.ConfigureChildComponents(&v0.ControlPlaneInstance{}, &v0.ControlPlaneInstance{}))

	for _, name := range DefaultComponentNames() {
		assert.Equal(t, ThreeportImageNamespace, componentByName(t, cpi, name).ImageNamespace, name)
	}
	assert.Equal(t, ThreeportAgent.ImageTag, cpi.Opts.AgentInfo.ImageTag)
}

func TestConfigureChildComponents_ChildTakesTheImagesItsParentRuns(t *testing.T) {
	cpi := NewInstaller()
	parent := &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			recorded("rest-api", "localhost:5001", "v0.7.0.abc123"),
			recorded("gateway-controller", "localhost:5001", "v0.7.0.abc123"),
			recorded("database-migrator", "localhost:5001", "v0.7.0.abc123"),
			{Name: "agent", ImageName: "forked-agent", ImageNamespace: "mirror.example.com", ImageTag: "v1"},
			// the parent's own settings that belong to the parent, not the child
			{Name: "aws-controller", ImagePullSecretName: "parent-secret", ServiceAccountName: "parent-sa", Enabled: util.Ptr(false)},
			// a component the parent added for itself, which is not a default component
			{Name: "parent-only", Additional: util.Ptr(true), ImageName: "p", ImageNamespace: "p", ImageTag: "p"},
			nil,
		},
	}

	require.NoError(t, cpi.ConfigureChildComponents(parent, &v0.ControlPlaneInstance{}))

	rest := componentByName(t, cpi, "rest-api")
	assert.Equal(t, "localhost:5001", rest.ImageNamespace)
	assert.Equal(t, "v0.7.0.abc123", rest.ImageTag)
	assert.Equal(t, ThreeportAPIImage, rest.ImageName, "an image name the parent did not record is kept")
	assert.Equal(t, "v0.7.0.abc123", componentByName(t, cpi, "gateway-controller").ImageTag)
	assert.Equal(t, "v0.7.0.abc123", cpi.Opts.DatabaseMigratorInfo.ImageTag)

	agent := componentByName(t, cpi, "agent")
	assert.Equal(t, "forked-agent", agent.ImageName)
	assert.Equal(t, "mirror.example.com", agent.ImageNamespace)

	// a component the parent did not record keeps the default
	assert.Equal(t, ThreeportImageNamespace, componentByName(t, cpi, "oci-controller").ImageNamespace)

	aws := componentByName(t, cpi, "aws-controller")
	assert.Empty(t, aws.ImagePullSecretName)
	assert.Equal(t, "aws-controller", aws.ServiceAccountName)
	assert.True(t, *aws.Enabled)

	for _, controller := range cpi.Opts.ControllerList {
		assert.NotEqual(t, "parent-only", controller.Name)
	}
}

func TestConfigureChildComponents_Precedence(t *testing.T) {
	cpi := NewInstaller()
	parent := &v0.ControlPlaneInstance{
		ImageTag: util.Ptr("parent-wide"),
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			recorded("rest-api", "parent-registry", "parent-recorded"),
			recorded("agent", "parent-registry", "parent-recorded"),
			recorded("aws-controller", "parent-registry", "parent-recorded"),
		},
	}
	child := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr("child-registry"),
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "aws-controller", ImageTag: "child-component"},
		},
	}

	require.NoError(t, cpi.ConfigureChildComponents(parent, child))

	// the child's component info wins over everything
	aws := componentByName(t, cpi, "aws-controller")
	assert.Equal(t, "child-component", aws.ImageTag)
	assert.Equal(t, "child-registry", aws.ImageNamespace)

	// the child's instance-wide image wins over the parent's
	assert.Equal(t, "child-registry", cpi.Opts.RestApiInfo.ImageNamespace)

	// the parent's instance-wide image wins over what the parent recorded
	assert.Equal(t, "parent-wide", cpi.Opts.RestApiInfo.ImageTag)
	assert.Equal(t, "parent-wide", cpi.Opts.AgentInfo.ImageTag)
	assert.Equal(t, "parent-wide", componentByName(t, cpi, "oci-controller").ImageTag)
}

func TestConfigureChildComponents_AppliesOverrides(t *testing.T) {
	cpi := NewInstaller()
	child := &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "gateway-controller", Enabled: util.Ptr(false)},
			{
				Name: "rest-api", ImageName: "my-api", ImageNamespace: "reg", ImageTag: "t",
				ServiceAccountName: "sa", ServiceResourceName: "svc", ImagePullSecretName: "pull",
			},
			{
				Name: "my-controller", Additional: util.Ptr(true),
				ImageName: "my-controller", ImageNamespace: "ghcr.io/me", ImageTag: "v1",
			},
		},
	}

	require.NoError(t, cpi.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child))

	assert.False(t, *componentByName(t, cpi, "gateway-controller").Enabled)
	assert.True(t, *componentByName(t, cpi, "aws-controller").Enabled)

	api := cpi.Opts.RestApiInfo
	assert.Equal(t, "my-api", api.ImageName)
	assert.Equal(t, "reg", api.ImageNamespace)
	assert.Equal(t, "t", api.ImageTag)
	assert.Equal(t, "sa", api.ServiceAccountName)
	assert.Equal(t, "svc", api.ServiceResourceName)
	assert.Equal(t, "pull", api.ImagePullSecretName)

	added := cpi.Opts.ControllerList[len(cpi.Opts.ControllerList)-1]
	assert.Equal(t, "my-controller", added.Name)
	require.NotNil(t, added.Enabled, "an added controller is installed unless disabled")
	assert.True(t, *added.Enabled)
	assert.NotSame(t, child.CustomComponentInfo[2], added)

	// none of it reaches the next child
	next := NewInstaller()
	require.NoError(t, next.ConfigureChildComponents(&v0.ControlPlaneInstance{}, &v0.ControlPlaneInstance{}))
	assert.True(t, *componentByName(t, next, "gateway-controller").Enabled)
	assert.Equal(t, ThreeportAPIImage, next.Opts.RestApiInfo.ImageName)
	assert.Len(t, next.Opts.ControllerList, len(ThreeportControllerList))
}

func TestConfigureChildComponents_RejectsUnknownNameBeforeChangingAnything(t *testing.T) {
	cpi := NewInstaller()
	child := &v0.ControlPlaneInstance{
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "agent", ImageTag: "changed"},
			{Name: "gateway-controler", ImageTag: "v1"},
		},
	}

	err := cpi.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "gateway-controler")
	assert.Equal(t, ThreeportAgent.ImageTag, cpi.Opts.AgentInfo.ImageTag)
	assert.Len(t, cpi.Opts.ControllerList, len(ThreeportControllerList))
}

func TestRecordedComponents(t *testing.T) {
	cpi := NewInstaller(CustomController(&v0.ControlPlaneComponent{
		Name: "mine", ImageName: "mine", ImageNamespace: "ghcr.io/me", ImageTag: "v1", Enabled: util.Ptr(true),
	}))

	got := cpi.RecordedComponents()

	require.Len(t, got, len(ThreeportControllerList)+1+3)
	byName := map[string]*v0.ControlPlaneComponent{}
	for _, component := range got {
		byName[component.Name] = component
	}
	for _, name := range DefaultComponentNames() {
		require.Contains(t, byName, name, "the record leaves out %s", name)
		assert.Nil(t, byName[name].Additional, name)
	}
	assert.True(t, *byName["mine"].Additional)

	// the record is a copy of what is installed, so what is recorded cannot
	// change the install, nor the install the record
	byName["agent"].ImageTag = "changed"
	assert.NotEqual(t, "changed", cpi.Opts.AgentInfo.ImageTag)
	cpi.Opts.RestApiInfo.ImageTag = "changed-later"
	assert.NotEqual(t, "changed-later", byName["rest-api"].ImageTag)
	assert.Nil(t, cpi.Opts.ControllerList[len(cpi.Opts.ControllerList)-1].Additional)
}

func TestRecordedComponents_AreAcceptedAsComponentInfo(t *testing.T) {
	// what a control plane records must be something its children can be
	// created from, additional components included
	cpi := NewInstaller(CustomController(&v0.ControlPlaneComponent{
		Name: "mine", ImageName: "mine", ImageNamespace: "ghcr.io/me", ImageTag: "v1",
	}))

	assert.NoError(t, ValidateComponentOverrides(cpi.RecordedComponents()))
}
