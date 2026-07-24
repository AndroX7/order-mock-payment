package webhook

import "errors"

// Sentinel errors returned by the webhook service. Transport code
// (handler.go) maps them to HTTP status codes and API error codes.
var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidRequest   = errors.New("invalid webhook request")
	ErrInvalidStatus    = errors.New("unsupported webhook status")
)
