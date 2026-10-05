package sandbox

import "fmt"

// Error is the base sandbox error.
type Error struct {
	Message string
	Details map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if len(e.Details) == 0 {
		return e.Message
	}
	return fmt.Sprintf("%s (%v)", e.Message, e.Details)
}

func newError(message string, details map[string]any) *Error {
	return &Error{Message: message, Details: details}
}

// NotFoundError is returned when a sandbox id is unknown.
type NotFoundError struct {
	SandboxID string
}

func (e *NotFoundError) Error() string {
	if e.SandboxID == "" {
		return "sandbox not found"
	}
	return "sandbox not found: " + e.SandboxID
}

func ErrNotFound(id string) error {
	return &NotFoundError{SandboxID: id}
}

// CommandError is returned when a sandbox command fails to start.
type CommandError struct {
	Message  string
	Command  string
	ExitCode int
}

func (e *CommandError) Error() string {
	return e.Message
}

func ErrCommand(message, command string, exitCode int) error {
	return &CommandError{Message: message, Command: command, ExitCode: exitCode}
}

// FileError is returned when a sandbox file operation fails.
type FileError struct {
	Message   string
	Path      string
	Operation string
}

func (e *FileError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Message + ": " + e.Path
}

func ErrFile(message, path, operation string) error {
	return &FileError{Message: message, Path: path, Operation: operation}
}

// AuthorizationError is returned when a role is denied sandbox execution.
type AuthorizationError struct {
	Role string
}

func (e *AuthorizationError) Error() string {
	if e.Role == "" {
		return "sandbox execution is not permitted for your role"
	}
	return "sandbox execution is not permitted for role " + e.Role
}

func ErrAuthorization(role string) error {
	return &AuthorizationError{Role: role}
}
