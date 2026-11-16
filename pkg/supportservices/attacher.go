package supportservices

import "net/http"

// componentAttacher is an API instance attached to one support services operator component.
type componentAttacher interface {
	GetFullyQualifiedType() string
	ObjectID() *uint
}

// objectUpdate is the save of an attached API instance after its component workload id is stored.
type objectUpdate[T any] func(apiClient *http.Client, apiAddr string, object T) (T, error)

// manifestEditor returns the edited component document.
// An instance that keeps the stock component manifest does not implement it.
type manifestEditor interface {
	EditManifest(base func() (string, error)) (string, error)
}
