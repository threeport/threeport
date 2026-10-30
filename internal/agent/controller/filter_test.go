package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/threeport/threeport/internal/agent"
	agentapi "github.com/threeport/threeport/pkg/agent/api/v1alpha1"
)

func workload(labels map[string]string, deleting bool) *agentapi.ThreeportWorkload {
	object := &agentapi.ThreeportWorkload{ObjectMeta: metav1.ObjectMeta{Labels: labels}}
	if deleting {
		now := metav1.Now()
		object.DeletionTimestamp = &now
	}

	return object
}

func admits(name string, object *agentapi.ThreeportWorkload) bool {
	p := ControlPlanePredicate(name)
	return p.Create(event.CreateEvent{Object: object}) &&
		p.Update(event.UpdateEvent{ObjectOld: object, ObjectNew: object}) &&
		p.Delete(event.DeleteEvent{Object: object})
}

func TestControlPlanePredicate(t *testing.T) {
	mine := agent.ThreeportWorkloadLabels("mine")
	theirs := agent.ThreeportWorkloadLabels("theirs")

	assert.True(t, admits("mine", workload(mine, false)))
	assert.False(t, admits("mine", workload(theirs, false)))
	assert.False(t, admits("mine", workload(theirs, true)), "another control plane's workload is not this agent's to finalize")
}

// a workload from before the label existed holds the agent's finalizer, so an
// agent that never sees it can never let its deletion finish
func TestControlPlanePredicateAdmitsUnlabeledWorkloads(t *testing.T) {
	assert.True(t, admits("mine", workload(nil, false)))
	assert.True(t, admits("mine", workload(nil, true)))
}

// an agent that has not been told its control plane keeps watching everything
func TestControlPlanePredicateWithoutName(t *testing.T) {
	assert.True(t, admits("", workload(agent.ThreeportWorkloadLabels("theirs"), false)))
	assert.True(t, admits("", workload(nil, false)))
}
