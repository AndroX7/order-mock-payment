package upload

import "errors"

// Sentinel errors returned by service, repository, and storage.
// Transport code (handler.go) maps them to HTTP status codes and API error codes.
var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrEmptyFile              = errors.New("uploaded file is empty")
	ErrFileTooLarge           = errors.New("uploaded file exceeds maximum size")
	ErrUnsupportedContentType = errors.New("unsupported file type")
	ErrUploadNotFound         = errors.New("upload not found")
	ErrInvalidStoragePath     = errors.New("invalid storage path")
)
