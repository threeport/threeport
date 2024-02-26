package models

import "go/ast"

// ControllerConfig contains the values at the controller scope.  A controller
// corresponds to all the models that are grouped together in a file in the API.
type ControllerConfig struct {
	ModelFilename         string
	PackageName           string
	ParsedModelFile       ast.File
	ControllerDomain      string
	ControllerDomainLower string
	ModelConfigs          []ModelConfig
	ReconcilerModels      []string
}

// ModelConfig contains the values for a particular model.
type ModelConfig struct {
	TypeName              string
	AllowDuplicateNames   bool
	AllowCustomMiddleware bool
	DbLoadAssociations    bool
	NameField             bool
	Reconciler            bool
	ReconciledField       bool

	// notification subjects
	CreateSubject string
	UpdateSubject string
	DeleteSubject string

	// handler names
	GetVersionHandlerName    string
	AddHandlerName           string
	AddMiddlewareFuncName    string
	GetAllHandlerName        string
	GetOneHandlerName        string
	GetMiddlewareFuncName    string
	PatchHandlerName         string
	PatchMiddlewareFuncName  string
	PutHandlerName           string
	PutMiddlewareFuncName    string
	DeleteHandlerName        string
	DeleteMiddlewareFuncName string
}
