package v0

import (
	"fmt"
	"net/http"
)

// Deletable defines the interface for operations that
// have been made to the Threeport API and may need to
// be deleted.
type Deletable interface {
	Delete(apiClient *http.Client, apiEndpoint string) error
}

// DeletableStack contains a list of operations that have been
// performed on the Threeport API.
type DeletableStack struct {
	Deletables []Deletable
}

// Push adds an deletable to the deletable stack.
func (r *DeletableStack) Push(deletable Deletable) {
	r.Deletables = append(r.Deletables, deletable)
}

// Pop removes an deletable from the deletable stack.
func (r *DeletableStack) Pop() Deletable {
	if len(r.Deletables) == 0 {
		return nil
	}
	lastIndex := len(r.Deletables) - 1
	deletable := r.Deletables[lastIndex]
	r.Deletables = r.Deletables[:lastIndex]
	return deletable
}

// CreateOperation is a function that creates a deletable and returns
// the deletable, an error message, and an error.
type CreateOperation func() (Deletable, string, error)

// CleanOnCreateError cleans up resources created during a create operation
func (r *DeletableStack) CleanOnCreateError(apiClient *http.Client, apiEndpoint string, createErr error) error {

	multiError := MultiError{}
	multiError.AppendError(createErr)

	for {
		var deletable Deletable
		if deletable = r.Pop(); deletable == nil {
			break
		}
		if err := deletable.Delete(apiClient, apiEndpoint); err != nil {
			multiError.AppendError(err)
		}
	}
	return multiError.Error()
}

// Create executes an operation and adds a deletable to the deletable stack.
func (r *DeletableStack) Create(apiClient *http.Client, apiEndpoint string, createOperation CreateOperation) error {
	deletable, errMsg, err := createOperation()
	if err != nil {
		return r.CleanOnCreateError(apiClient, apiEndpoint, fmt.Errorf("%s: %w", errMsg, err))
	}
	r.Push(deletable)

	return nil
}
