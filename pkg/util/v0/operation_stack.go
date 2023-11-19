package v0

import (
	"fmt"
	"net/http"
	// iface "github.com/threeport/threeport/pkg/interfaces/v0"
)

// Deletable defines the interface for operations that
// have been made to the Threeport API and may need to
// be deleted.
type Deletable interface {
	Delete(apiClient *http.Client, apiEndpoint string) error
}

// CreateOperation is a function that creates a deletable and returns
// the deletable, an error message, and an error.
type CreateOperation func() (Deletable, string, error)

// OperationStack contains a list of operations that have been
// performed on the Threeport API.
type OperationStack struct {
	Deletables       []Deletable
	CreateOperations []CreateOperation
	ApiClient        *http.Client
	ApiEndpoint      string
}

// AppendCreateOperation adds a create operation to the operation stack.
func (r *OperationStack) AppendCreateOperation(createOperation CreateOperation) {
	r.CreateOperations = append(r.CreateOperations, createOperation)
}

// ExecuteCreateOperations executes all create operations in the operation stack.
func (r *OperationStack) ExecuteCreateOperations() error {
	for _, createOperation := range r.CreateOperations {
		if err := r.create(createOperation); err != nil {
			return err
		}
	}
	return nil
}

// push adds an deletable to the deletable stack.
func (r *OperationStack) push(deletable Deletable) {
	r.Deletables = append(r.Deletables, deletable)
}

// pop removes an deletable from the deletable stack.
func (r *OperationStack) pop() Deletable {
	if len(r.Deletables) == 0 {
		return nil
	}
	lastIndex := len(r.Deletables) - 1
	deletable := r.Deletables[lastIndex]
	r.Deletables = r.Deletables[:lastIndex]
	return deletable
}

// cleanOnCreateError cleans up resources created during a create operation
func (r *OperationStack) cleanOnCreateError(createErr error) error {

	multiError := MultiError{}
	multiError.AppendError(createErr)

	for {
		deletable := r.pop()
		if any(deletable) == nil {
			break
		}
		switch v := any(deletable).(type) {
		case Deletable:
			if err := v.Delete(r.ApiClient, r.ApiEndpoint); err != nil {
				multiError.AppendError(err)
			}
			continue
		}

	}
	return multiError.Error()
}

// create executes an operation and adds a deletable to the deletable stack.
func (r *OperationStack) create(createOperation CreateOperation) error {
	deletable, errMsg, err := createOperation()
	if err != nil {
		return r.cleanOnCreateError(fmt.Errorf("failed to create %s: %w", errMsg, err))
	}
	r.push(deletable)

	return nil
}
