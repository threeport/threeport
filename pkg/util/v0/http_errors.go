package v0

import "net/http"

// HttpError represents an error with an associated HTTP status code
type HttpError struct {
	// Message is the error message
	Message string
	// StatusCode is the HTTP status code that should be returned
	StatusCode int
}

// Error returns the error message
func (e *HttpError) Error() string {
	return e.Message
}

// GetStatusCode returns the HTTP status code
func (e *HttpError) GetStatusCode() int {
	return e.StatusCode
}

// NewBadRequestError creates a new HttpError with status 400 Bad Request
func NewBadRequestError(message string) *HttpError {
	return &HttpError{
		Message:    message,
		StatusCode: http.StatusBadRequest,
	}
}

// NewUnauthorizedError creates a new HttpError with status 401 Unauthorized
func NewUnauthorizedError(message string) *HttpError {
	return &HttpError{
		Message:    message,
		StatusCode: http.StatusUnauthorized,
	}
}

// NewForbiddenError creates a new HttpError with status 403 Forbidden
func NewForbiddenError(message string) *HttpError {
	return &HttpError{
		Message:    message,
		StatusCode: http.StatusForbidden,
	}
}

// NewNotFoundError creates a new HttpError with status 404 Not Found
func NewNotFoundError(message string) *HttpError {
	return &HttpError{
		Message:    message,
		StatusCode: http.StatusNotFound,
	}
}

// NewConflictError creates a new HttpError with status 409 Conflict
func NewConflictError(message string) *HttpError {
	return &HttpError{
		Message:    message,
		StatusCode: http.StatusConflict,
	}
}
