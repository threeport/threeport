package v0

import (
	"fmt"
)

// Operation contains a create, replace and delete function for a Threeport API object.
type Operation struct {
	Name    string
	Get     func() error
	Create  func() error
	Replace func(string) error
	Delete  func() error
}

// Operations contains a list of operations that have been
// performed on the Threeport API.
type Operations struct {
	Operations []*Operation
}

// AppendOperation adds a create operation to the operation stack.
func (r *Operations) AppendOperation(operation Operation) {
	r.Operations = append(r.Operations, &operation)
}

// Get executes all get operations in the operation stack.
func (r *Operations) Get() error {
	for _, operation := range r.Operations {
		err := operation.Get()
		if err != nil {
			return fmt.Errorf("failed to get %s: %w", operation.Name, err)
		}
	}
	return nil
}

// Create executes all create operations in the operation stack.
func (r *Operations) Create() error {
	for index, operation := range r.Operations {
		err := operation.Create()
		if err != nil {
			return r.cleanOnCreateError(index-1, fmt.Errorf("failed to create %s: %w", operation.Name, err))
		}
	}
	return nil
}

// Replace executes all replace operations in the operation stack.
func (r *Operations) Replace(name string) error {
	for _, operation := range r.Operations {
		err := operation.Replace(name)
		if err != nil {
			return fmt.Errorf("failed to replace %s: %w", operation.Name, err)
		}
	}
	return nil
}

// Delete executes all delete operations in the operation stack.
func (r *Operations) Delete() error {
	return r.delete(len(r.Operations) - 1)
}

// cleanOnCreateError cleans up resources created during a create operation
func (r *Operations) cleanOnCreateError(startIndex int, createErr error) error {

	multiError := MultiError{}
	multiError.AppendError(createErr)

	if err := r.delete(startIndex); err != nil {
		multiError.AppendError(err)
	}

	return multiError.Error()
}

// delete deletes operations in the operation slice
// starting from the startIndex and iterating backwards.
func (r *Operations) delete(startIndex int) error {
	multiError := MultiError{}
	for i := startIndex; i >= 0; i-- {
		operation := r.Operations[i]
		err := operation.Delete()
		if err != nil {
			multiError.AppendError(fmt.Errorf("failed to delete %s: %w", operation.Name, err))
		}
	}
	return multiError.Error()
}
