package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThreeportWorkloadLabels(t *testing.T) {
	assert.Equal(
		t,
		map[string]string{"control-plane.threeport.io/control-plane-name": "cp"},
		ThreeportWorkloadLabels("cp"),
	)
	assert.Nil(t, ThreeportWorkloadLabels(""))
}
