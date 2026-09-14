package v0

import (
	"errors"
	"strings"
	"testing"
)

// TestUnmetPrerequisitesReturnsNilWhenEveryArgIsNil covers an empty join.
func TestUnmetPrerequisitesReturnsNilWhenEveryArgIsNil(t *testing.T) {
	// join two nil errors
	got := UnmetPrerequisites("prefix:", nil, nil)
	// assert nil
	if got != nil {
		t.Errorf("UnmetPrerequisites() = %v, want nil", got)
	}
}

// TestUnmetPrerequisitesPrefixesJoinedErrors covers prefix plus Join.
func TestUnmetPrerequisitesPrefixesJoinedErrors(t *testing.T) {
	// join two independent errors
	got := UnmetPrerequisites("prefix:", errors.New("a"), errors.New("b"))
	if got == nil {
		t.Fatal("UnmetPrerequisites() = nil, want prefixed join")
	}
	// assert the prefix and both messages
	s := got.Error()
	if !strings.HasPrefix(s, "prefix:\n") {
		t.Errorf("UnmetPrerequisites() = %q, want prefix", s)
	}
	if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Errorf("UnmetPrerequisites() = %q, want both errors", s)
	}
}
