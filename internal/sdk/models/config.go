package models

import "go/ast"

// ControllerConfig contains the values at the controller scope.  A controller
// corresponds to all the models that are grouped together in a file in the API.
type ControllerConfig struct {
	ModelFilename          string
	PackageName            string
	ParsedModelFile        ast.File
	ControllerDomain       string
	ControllerDomainLower  string
	ModelConfigs           []ModelConfig
	ReconcilerModels       []string
	TptctlModels           []string
	TptctlConfigPathModels []string
	ApiVersion             string
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

	// if true, generate tptctl commands for the model
	TptctlCommands bool

	// if true, the config for the object, references another file and should
	// have code that includes passing the config path to config package object
	TptctlConfigPath bool

	// only applied to definition objects - if true, there is a corresponding
	// instance object
	DefinedInstance bool

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
