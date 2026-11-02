package supportservices

import "net/http"

// componentCaller is an API object that records one shared workload.
type componentCaller interface {
	GetFullyQualifiedType() string
	ObjectID() *uint
}

// objectUpdate saves the API object that records a component workload.
type objectUpdate[T any] func(apiClient *http.Client, apiAddr string, object T) (T, error)

// manifestEditor returns the component document, including any object the caller adds.
// A caller that leaves the document unchanged does not implement it.
type manifestEditor interface {
	EditManifest(base func() (string, error)) (string, error)
}
