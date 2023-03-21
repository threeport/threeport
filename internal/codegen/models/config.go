package models

import "go/ast"

// ControllerConfig contains the values at the controller scope.  A controller
// corresponds to all the models that are grouped together in a file in the API.
type ControllerConfig struct {
	ModelFilename         string
	ParsedModelFile       ast.File
	ControllerDomain      string
	ControllerDomainLower string
	ModelConfigs          []ModelConfig
}

// ModelConfig contains the values for a particular model.
type ModelConfig struct {
	TypeName string

	// notification subjects
	CreateSubject string
	UpdateSubject string
	DeleteSubject string

	// handler names
	GetVersionHandlerName string
	AddHandlerName        string
	GetAllHandlerName     string
	GetOneHandlerName     string
	PatchHandlerName      string
	PutHandlerName        string
	DeleteHandlerName     string
}
