package webhook

import "errors"

var (
	ErrInvalidSignature = errors.New("invalid signature")
	ErrInvalidRequest   = errors.New("invalid webhook request")
	ErrInvalidStatus    = errors.New("unsupported webhook status")
)
