package upload

import "errors"

var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrEmptyFile              = errors.New("uploaded file is empty")
	ErrFileTooLarge           = errors.New("uploaded file exceeds maximum size")
	ErrUnsupportedContentType = errors.New("unsupported file type")
	ErrUploadNotFound         = errors.New("upload not found")
	ErrInvalidStoragePath     = errors.New("invalid storage path")
)
