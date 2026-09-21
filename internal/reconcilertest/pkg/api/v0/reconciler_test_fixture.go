// Package v0 is the API model for the reconciler test fixture.
package v0

import (
	tpapi "github.com/threeport/threeport/pkg/api/v0"
)

// The fixture is a miniature module. The generators that emit API
// methods, the client, and the reconciler run over these types the
// same way they run over a real module, so a generator change fails
// here. This file is the generator input; the rest of the package is
// generated.
//
// persist:"false" on any field excludes that field from the database
// and makes the generated reconciler skip its pre-reconcile reload,
// because the notification payload is the only remaining copy. The
// second type below is the object that path is generated for.

// ReconcilerTestInstance is a fixture object the generated reconciler
// dispatches on after reloading it from the API.
type ReconcilerTestInstance struct {
	tpapi.Common         `swaggerignore:"true" mapstructure:",squash"`
	tpapi.Instance       `mapstructure:",squash"`
	tpapi.Reconciliation `mapstructure:",squash"`

	// The status of the instance.
	Status *string `validate:"optional"`
}

// ReconcilerTestVolatileInstance is a fixture object whose generated
// reconciler dispatches from the notification payload.
type ReconcilerTestVolatileInstance struct {
	tpapi.Common         `swaggerignore:"true" mapstructure:",squash"`
	tpapi.Instance       `mapstructure:",squash"`
	tpapi.Reconciliation `mapstructure:",squash"`

	// The payload delivered with the notification and never stored.
	Data *string `validate:"optional" persist:"false"`
}
