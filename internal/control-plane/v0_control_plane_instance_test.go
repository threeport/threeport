package controlplane

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/threeport/threeport/pkg/api/v0"
	threeport "github.com/threeport/threeport/pkg/threeport-installer/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// TestUpdatedSkipsAControlPlaneThisAPIDoesNotServe returns before reconciling a row this API does not drive.
// A parent drives a child it manages, and the child's own row stays idle.
func TestUpdatedSkipsAControlPlaneThisAPIDoesNotServe(t *testing.T) {
	// skip a row marked for another API
	delay, err := v0ControlPlaneInstanceUpdated(nil, &v0.ControlPlaneInstance{IsSelf: util.Ptr(false)}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	// skip a row that has not recorded which API serves it
	delay, err = v0ControlPlaneInstanceUpdated(nil, &v0.ControlPlaneInstance{}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)

	// the child's own row is owned by its parent, so this API does not reconcile it
	parentID := uint(7)
	delay, err = v0ControlPlaneInstanceUpdated(nil, &v0.ControlPlaneInstance{
		IsSelf:                       util.Ptr(true),
		ParentControlPlaneInstanceID: &parentID,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(0), delay)
}

// TestAppliesControlPlane decides which control plane instance this API reconciles.
func TestAppliesControlPlane(t *testing.T) {
	// a parent reconciles the child row, and a root reconciles itself
	parentID := uint(7)
	cases := []struct {
		name     string
		instance *v0.ControlPlaneInstance
		want     bool
	}{
		{name: "root", instance: &v0.ControlPlaneInstance{IsSelf: util.Ptr(true)}, want: true},
		{
			name: "managed child",
			instance: &v0.ControlPlaneInstance{
				IsSelf:                       util.Ptr(false),
				ParentControlPlaneInstanceID: &parentID,
			},
			want: true,
		},
		{
			name: "owned self",
			instance: &v0.ControlPlaneInstance{
				IsSelf:                       util.Ptr(true),
				ParentControlPlaneInstanceID: &parentID,
			},
			want: false,
		},
		{name: "foreign copy", instance: &v0.ControlPlaneInstance{IsSelf: util.Ptr(false)}, want: false},
		{name: "unset", instance: &v0.ControlPlaneInstance{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, appliesControlPlane(tc.instance))
		})
	}
}

// TestClientEndpointReplace reports when a stored address should become the public one.
func TestClientEndpointReplace(t *testing.T) {
	// an empty address and the in-cluster name are replaced
	internal := "threeport-api-server.primary-01:443"
	cases := []struct {
		name     string
		saved    string
		internal string
		want     bool
	}{
		{name: "empty", saved: "", internal: internal, want: true},
		{name: "in-cluster name", saved: internal, internal: internal, want: true},
		{name: "public address", saved: "34.132.193.86:443", internal: internal, want: false},
		{name: "other host", saved: "api.example.com:443", internal: internal, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, clientEndpointReplace(tc.saved, tc.internal))
		})
	}
}

// TestAPIEndpointUpdate keeps the internal address when the service is not a load balancer.
func TestAPIEndpointUpdate(t *testing.T) {
	// a ClusterIP service has no address to wait for
	internal := "threeport-api-server.primary-01:443"
	public := "34.132.193.86:443"
	cases := []struct {
		name        string
		saved       string
		serviceType string
		want        string
		readLB      bool
	}{
		{name: "empty clusterip", saved: "", serviceType: "ClusterIP", want: internal, readLB: false},
		{name: "internal clusterip", saved: internal, serviceType: "ClusterIP", want: internal, readLB: false},
		{name: "empty type", saved: "", serviceType: "", want: internal, readLB: false},
		{name: "internal load balancer", saved: internal, serviceType: "LoadBalancer", want: "", readLB: true},
		{name: "public load balancer", saved: public, serviceType: "LoadBalancer", want: public, readLB: false},
		{name: "public clusterip", saved: public, serviceType: "ClusterIP", want: public, readLB: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, readLB := apiEndpointUpdate(tc.saved, internal, tc.serviceType)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.readLB, readLB)
		})
	}
}

// TestUnfinishedChild reports a child whose create never stored a parent.
func TestUnfinishedChild(t *testing.T) {
	// a root, a copied genesis, and a finished child stay out
	parentID := uint(7)
	acknowledged := time.Now()
	cases := []struct {
		name     string
		instance *v0.ControlPlaneInstance
		want     bool
	}{
		{
			name: "acknowledged child with no parent",
			instance: &v0.ControlPlaneInstance{
				IsSelf:               util.Ptr(false),
				Genesis:              util.Ptr(false),
				CreationAcknowledged: &acknowledged,
			},
			want: true,
		},
		{
			name: "root",
			instance: &v0.ControlPlaneInstance{
				IsSelf:               util.Ptr(true),
				CreationAcknowledged: &acknowledged,
			},
			want: false,
		},
		{
			name: "copied genesis",
			instance: &v0.ControlPlaneInstance{
				IsSelf:               util.Ptr(false),
				Genesis:              util.Ptr(true),
				CreationAcknowledged: &acknowledged,
			},
			want: false,
		},
		{
			name: "parent already stored",
			instance: &v0.ControlPlaneInstance{
				IsSelf:                       util.Ptr(false),
				ParentControlPlaneInstanceID: &parentID,
				CreationAcknowledged:         &acknowledged,
			},
			want: false,
		},
		{
			name:     "not acknowledged",
			instance: &v0.ControlPlaneInstance{IsSelf: util.Ptr(false)},
			want:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, unfinishedChild(tc.instance))
		})
	}
}

// A child records what it runs so that its own children can default to the
// same images. Its children apply an instance-wide image over whatever its
// record says, so the record cannot keep one next to components that are
// exceptions to it.
func TestRecordResolvedComponents_GrandchildKeepsTheChildsExceptions(t *testing.T) {
	child := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr("wide-registry"),
		ImageTag:       util.Ptr("wide"),
		CustomComponentInfo: []*v0.ControlPlaneComponent{
			{Name: "agent", ImageNamespace: "special-registry", ImageTag: "special"},
		},
	}
	childInstaller := threeport.NewInstaller()
	require.NoError(t, childInstaller.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child))
	require.Equal(t, "special", childInstaller.Opts.AgentInfo.ImageTag)
	require.Equal(t, "wide", childInstaller.Opts.RestApiInfo.ImageTag)

	// what the child writes as its own self record
	record := *child
	recordResolvedComponents(&record, childInstaller)

	assert.Nil(t, record.ImageNamespace)
	assert.Nil(t, record.ImageTag)

	// a control plane created from the child
	grandchildInstaller := threeport.NewInstaller()
	require.NoError(t, grandchildInstaller.ConfigureChildComponents(&record, &v0.ControlPlaneInstance{}))

	assert.Equal(t, "special", grandchildInstaller.Opts.AgentInfo.ImageTag)
	assert.Equal(t, "special-registry", grandchildInstaller.Opts.AgentInfo.ImageNamespace)
	assert.Equal(t, "wide", grandchildInstaller.Opts.RestApiInfo.ImageTag)
	assert.Equal(t, "wide-registry", grandchildInstaller.Opts.RestApiInfo.ImageNamespace)

	// the child's own instance is left as it was asked for
	assert.Equal(t, "wide", *child.ImageTag)
}

// TestSelectInstanceControllersKeepsNamedGroup checks that gcp keeps only gcp-controller.
// The image applied before the select stays on that controller.
func TestSelectInstanceControllersKeepsNamedGroup(t *testing.T) {
	childInstaller := threeport.NewInstaller()
	child := &v0.ControlPlaneInstance{
		ImageNamespace: util.Ptr("ghcr.io/child"),
		Apis:           util.Ptr("gcp"),
	}
	require.NoError(t, childInstaller.ConfigureChildComponents(&v0.ControlPlaneInstance{}, child))

	// keep gcp-controller after images are set
	require.NoError(t, selectInstanceControllers(childInstaller, child.Apis))

	require.Len(t, childInstaller.Opts.ControllerList, 1)
	assert.Equal(t, "gcp-controller", childInstaller.Opts.ControllerList[0].Name)
	assert.Equal(t, "ghcr.io/child", childInstaller.Opts.ControllerList[0].ImageNamespace)
}

// TestSelectInstanceControllersNoneClearsTheList checks that none installs no controllers.
func TestSelectInstanceControllersNoneClearsTheList(t *testing.T) {
	childInstaller := threeport.NewInstaller()

	// none clears the controller list
	require.NoError(t, selectInstanceControllers(childInstaller, util.Ptr("none")))

	assert.Nil(t, childInstaller.Opts.ControllerList)
}

// TestSelectInstanceControllersNilLeavesTheList checks that an omitted Apis installs every controller.
func TestSelectInstanceControllersNilLeavesTheList(t *testing.T) {
	childInstaller := threeport.NewInstaller()
	before := len(childInstaller.Opts.ControllerList)

	// leave the full list when Apis is omitted
	require.NoError(t, selectInstanceControllers(childInstaller, nil))

	assert.Len(t, childInstaller.Opts.ControllerList, before)
}

// TestSelectInstanceControllersRejectsUnknownGroup checks that an unknown group returns an error.
func TestSelectInstanceControllersRejectsUnknownGroup(t *testing.T) {
	childInstaller := threeport.NewInstaller()

	// an unknown group fails and leaves the list in place
	err := selectInstanceControllers(childInstaller, util.Ptr("not-a-group"))

	require.Error(t, err)
	assert.NotEmpty(t, childInstaller.Opts.ControllerList)
}

// TestClientCertificateRecorded checks that a saved certificate is kept and an empty one is not.
func TestClientCertificateRecorded(t *testing.T) {
	empty := ""
	recorded := &v0.ControlPlaneInstance{
		CACert:     util.Ptr("ca"),
		ClientCert: util.Ptr("cert"),
		ClientKey:  util.Ptr("key"),
	}

	// a saved certificate is reused on a retry
	assert.True(t, clientCertificateRecorded(recorded))
	assert.False(t, clientCertificateRecorded(&v0.ControlPlaneInstance{}))
	assert.False(t, clientCertificateRecorded(&v0.ControlPlaneInstance{
		CACert:     &empty,
		ClientCert: util.Ptr("cert"),
		ClientKey:  util.Ptr("key"),
	}))
}
