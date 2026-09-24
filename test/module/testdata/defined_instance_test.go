package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	util "github.com/threeport/threeport/pkg/util/v0"
)

// This file is tracked at test/module/testdata and copied into the generated
// config package before the module is built. The code it covers is emitted by
// the SDK and so is not in this repository, and type-checking the generated
// module does not reach it: the failure it guards is a panic at run time.

// TestMapToDefinedInstances_PairsByName covers the pairing every
// defined-instance Get performs. The instance values are what the generated
// Get produces: a name, an age, and the definition the row belongs to.
func TestMapToDefinedInstances_PairsByName(t *testing.T) {
	definitions := []WidgetDefinitionConfig{
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("first")}},
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("second")}},
	}
	instances := []WidgetInstanceConfig{
		{WidgetInstance: WidgetInstanceValues{
			Name:             util.Ptr("second"),
			WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("second")},
			Age:              util.Ptr("2d"),
		}},
	}

	var configs *[]WidgetConfig
	require.NotPanics(t, func() {
		configs = mapToWidgetDefinedInstances(&definitions, &instances)
	})

	require.Len(t, *configs, 1, "an instance pairs with the definition sharing its name")
	assert.Equal(t, "second", *(*configs)[0].Widget.Name)
	assert.Equal(t, "2d", *(*configs)[0].Widget.Age)
}

// TestMapToDefinedInstances_SkipsNilNames covers the values a name can take on
// the way through. A config assembled by hand, or an object whose name was
// never set, reaches this with a nil pointer where the pairing reads a string.
func TestMapToDefinedInstances_SkipsNilNames(t *testing.T) {
	definitions := []WidgetDefinitionConfig{
		{WidgetDefinition: WidgetDefinitionValues{}},
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("named")}},
	}
	instances := []WidgetInstanceConfig{
		{WidgetInstance: WidgetInstanceValues{}},
		{WidgetInstance: WidgetInstanceValues{Name: util.Ptr("no-definition-for-this")}},
		{WidgetInstance: WidgetInstanceValues{
			Name:             util.Ptr("named"),
			WidgetDefinition: &WidgetDefinitionValues{},
		}},
	}

	var configs *[]WidgetConfig
	require.NotPanics(t, func() {
		configs = mapToWidgetDefinedInstances(&definitions, &instances)
	})
	assert.Empty(t, *configs, "neither instance is half of a defined instance")
}

// TestMapToDefinedInstances_TakesOneDefinitionPerInstance covers the break: an
// instance pairs once, however many definitions carry its name.
func TestMapToDefinedInstances_TakesOneDefinitionPerInstance(t *testing.T) {
	definitions := []WidgetDefinitionConfig{
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("dup")}},
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("dup")}},
	}
	instances := []WidgetInstanceConfig{
		{WidgetInstance: WidgetInstanceValues{
			Name:             util.Ptr("dup"),
			WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("dup")},
		}},
	}

	configs := mapToWidgetDefinedInstances(&definitions, &instances)
	assert.Len(t, *configs, 1)
}

// TestMapToDefinedInstances_RejectsAMismatchedReference is what checking the
// reference buys. Two objects can carry one name while the instance belongs to
// a different definition, and pairing them would report a relationship the
// database does not have.
func TestMapToDefinedInstances_RejectsAMismatchedReference(t *testing.T) {
	definitions := []WidgetDefinitionConfig{
		{WidgetDefinition: WidgetDefinitionValues{Name: util.Ptr("shared")}},
	}
	instances := []WidgetInstanceConfig{
		{WidgetInstance: WidgetInstanceValues{
			Name:             util.Ptr("shared"),
			WidgetDefinition: &WidgetDefinitionValues{Name: util.Ptr("somewhere-else")},
		}},
	}

	configs := mapToWidgetDefinedInstances(&definitions, &instances)
	assert.Empty(t, *configs, "a name in common is not a relationship")
}
