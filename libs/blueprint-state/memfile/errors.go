package memfile

import (
	"fmt"
)

// Error is a custom error type that provides errors specific to
// the memfile (in-memory, persisted to file) implementation of the state.Container interface.
type Error struct {
	ReasonCode ErrorReasonCode
	Err        error
}

func (e *Error) Error() string {
	return fmt.Sprintf("memfile state error (%s): %s", e.ReasonCode, e.Err)
}

// ErrorReasonCode is an enum of possible error reasons that can be returned by the memfile implementation.
type ErrorReasonCode string

const (
	// ErrorReasonCodeMalformedStateFile is the error code that is used when
	// a state file is malformed,
	// this could be due to a file being corrupted or a mismatch between
	// the index and the actual state file.
	ErrorReasonCodeMalformedStateFile ErrorReasonCode = "malformed_state_file"

	// ErrorReasonCodeMalformedState is the error code that is used when
	// the in-memory state is malformed, usually when the instance associated
	// with a resource or link no longer exists but the resource or link
	// still exists.
	ErrorReasonCodeMalformedState ErrorReasonCode = "malformed_state"

	// ErrorReasonCodeExportNotFound is the error code that is used when
	// an export is not found in the state.
	ErrorReasonCodeExportNotFound ErrorReasonCode = "export_not_found"
)

func errMalformedState(message string) error {
	return &Error{
		ReasonCode: ErrorReasonCodeMalformedState,
		Err:        fmt.Errorf("malformed state: %s", message),
	}
}

func errMalformedStateFile(message string) error {
	return &Error{
		ReasonCode: ErrorReasonCodeMalformedStateFile,
		Err:        fmt.Errorf("malformed state file: %s", message),
	}
}

func errExportNotFound(instanceID string, exportName string) error {
	return &Error{
		ReasonCode: ErrorReasonCodeExportNotFound,
		Err:        fmt.Errorf("export %q not found in instance %q", exportName, instanceID),
	}
}
