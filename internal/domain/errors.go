package domain

import "fmt"

type ErrorCode string

const (
	ErrProtocol    ErrorCode = "protocol"
	ErrVersion     ErrorCode = "version"
	ErrAuth        ErrorCode = "auth"
	ErrBusy        ErrorCode = "busy"
	ErrTimeout     ErrorCode = "timeout"
	ErrClosed      ErrorCode = "closed"
	ErrTransport   ErrorCode = "transport"
	ErrUnsupported ErrorCode = "unsupported"
	ErrConfig      ErrorCode = "config"
	ErrInternal    ErrorCode = "internal"
)

type GhostwireError struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *GhostwireError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *GhostwireError) Unwrap() error {
	return e.Cause
}

func NewError(code ErrorCode, message string) *GhostwireError {
	return &GhostwireError{Code: code, Message: message}
}

func WrapError(err error, code ErrorCode, message string) *GhostwireError {
	return &GhostwireError{Code: code, Message: message, Cause: err}
}

func ToGhostwireError(err error, fallback ErrorCode) *GhostwireError {
	if err == nil {
		return nil
	}
	if gw, ok := err.(*GhostwireError); ok {
		return gw
	}
	return &GhostwireError{Code: fallback, Message: err.Error(), Cause: err}
}
