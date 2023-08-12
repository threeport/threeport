package gateway

import (
	"github.com/go-logr/logr"
	v0 "github.com/threeport/threeport/pkg/api/v0"
	controller "github.com/threeport/threeport/pkg/controller/v0"
)

// domainNameDefinitionCreated performs reconciliation when a domain name definition
// has been created.
func domainNameDefinitionCreated(
	r *controller.Reconciler,
	domainNameDefinition *v0.DomainNameDefinition,
	log *logr.Logger,
) error {

	return nil
}

// domainNameDefinitionUpdated performs reconciliation when a domain name definition
// has been updated.
func domainNameDefinitionUpdated(
	r *controller.Reconciler,
	domainNameDefinition *v0.DomainNameDefinition,
	log *logr.Logger,
) error {

	return nil
}

// domainNameDefinitionDeleted performs reconciliation when a domain name definition
// has been deleted.
func domainNameDefinitionDeleted(
	r *controller.Reconciler,
	domainNameDefinition *v0.DomainNameDefinition,
	log *logr.Logger,
) error {

	return nil
}