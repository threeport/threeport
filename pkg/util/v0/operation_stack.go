package v0

import (
	"fmt"
	// iface "github.com/threeport/threeport/pkg/interfaces/v0"
)

// // Deletable defines the interface for operations that
// // have been made to the Threeport API and may need to
// // be deleted.
// type Deletable interface {
// 	Delete(apiClient *http.Client, apiEndpoint string) error
// }

// CreateOperation is a function that creates a deletable and returns
// the deletable, an error message, and an error.
// type CreateOperation func() (Deletable, string, error)

type Operation struct {
	ObjectName string
	Create     func() error
	Update     func() error
	Delete     func() error
}

// OperationSlice contains a list of operations that have been
// performed on the Threeport API.
type OperationSlice struct {
	Operations []*Operation
}

// AppendOperation adds a create operation to the operation stack.
func (r *OperationSlice) AppendOperation(operation Operation) {
	r.Operations = append(r.Operations, &operation)
}

// ExecuteCreateOperations executes all create operations in the operation stack.
func (r *OperationSlice) ExecuteCreateOperations() error {
	for index, operation := range r.Operations {
		err := operation.Create()
		if err != nil {
			return r.cleanOnCreateError(index-1, fmt.Errorf("failed to create %s: %w", operation.ObjectName, err))
		}
	}
	return nil
}

// ExecuteUpdateOperations executes all delete operations in the operation stack.
func (r *OperationSlice) ExecuteDeleteOperations() error {
	return r.delete(len(r.Operations) - 1)
}

// cleanOnCreateError cleans up resources created during a create operation
func (r *OperationSlice) cleanOnCreateError(startIndex int, createErr error) error {

	multiError := MultiError{}
	multiError.AppendError(createErr)

	if err := r.delete(startIndex); err != nil {
		multiError.AppendError(err)
	}

	return multiError.Error()
}

// delete deletes all operations in the operation stack.
func (r *OperationSlice) delete(startIndex int) error {
	multiError := MultiError{}
	for i := startIndex; i >= 0; i-- {
		operation := r.Operations[i]
		err := operation.Delete()
		if err != nil {
			multiError.AppendError(fmt.Errorf("failed to delete %s: %w", operation.ObjectName, err))
		}
	}
	return multiError.Error()
}
