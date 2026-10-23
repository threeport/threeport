package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

func TestCacheOptionsScopesToControlPlane(t *testing.T) {
	options := CacheOptions("mine")
	require.Len(t, options.ByObject, 1)

	var selector labels.Selector
	for object, byObject := range options.ByObject {
		assert.IsType(t, &agentapi.ThreeportWorkload{}, object)
		selector = byObject.Label
	}

	assert.True(t, selector.Matches(labels.Set(agent.ThreeportWorkloadLabels("mine"))))
	assert.False(t, selector.Matches(labels.Set(agent.ThreeportWorkloadLabels("theirs"))))
	assert.False(t, selector.Matches(labels.Set{}), "an unlabeled workload is no one's")
}

// an agent that has not been told its control plane keeps watching everything
func TestCacheOptionsUnscopedWithoutName(t *testing.T) {
	assert.Empty(t, CacheOptions("").ByObject)
}
