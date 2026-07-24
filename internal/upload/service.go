package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/order"
)

// OrderService is the minimal order-domain surface the upload service
// needs. Consumer-owned interface; *order.Service satisfies it.
type OrderService interface {
	Get(ctx context.Context, userID, orderID uuid.UUID) (*order.Order, error)
}

// allowedMIMEs maps accepted content types to the extension used when
// generating the server-side filename. MIME is detected from the actual
// bytes, never trusted from the client Content-Type header.
var allowedMIMEs = map[string]string{
	"application/pdf": "pdf",
	"image/png":       "png",
	"image/jpeg":      "jpg",
}

// Service orchestrates file uploads: ownership check → validation →
// storage → metadata persistence.
type Service struct {
	repo    Repository
	storage Storage
	orders  OrderService
	maxSize int64
}

func NewService(repo Repository, storage Storage, orders OrderService, maxSize int64) *Service {
	return &Service{repo: repo, storage: storage, orders: orders, maxSize: maxSize}
}

// Create validates and persists an uploaded file attached to the given
// order. The order must be owned by userID.
func (s *Service) Create(ctx context.Context, userID, orderID uuid.UUID, file multipart.File, size int64) (*Upload, error) {
	// Ownership: order must exist and belong to userID.
	if _, err := s.orders.Get(ctx, userID, orderID); err != nil {
		if errors.Is(err, order.ErrOrderNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if size <= 0 {
		return nil, ErrEmptyFile
	}
	if size > s.maxSize {
		return nil, ErrFileTooLarge
	}

	// MIME detection from the first 512 bytes of the actual file, not
	// from the client-supplied Content-Type header.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("read file: %w", err)
	}
	mime := http.DetectContentType(head[:n])
	ext, ok := allowedMIMEs[mime]
	if !ok {
		return nil, ErrUnsupportedContentType
	}

	// Rewind so storage sees the full file.
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek file: %w", err)
	}

	// Generate a server-side filename. Never trust the original name.
	filename := uuid.NewString() + "." + ext

	path, err := s.storage.Save(ctx, file, filename)
	if err != nil {
		return nil, fmt.Errorf("save file: %w", err)
	}

	u := &Upload{
		OrderID:     orderID,
		Filename:    filename,
		ContentType: mime,
		Size:        size,
		Path:        path,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

// Get returns the upload if it exists and its underlying order is
// owned by userID. Otherwise ErrUploadNotFound (uniform 404).
func (s *Service) Get(ctx context.Context, userID, uploadID uuid.UUID) (*Upload, error) {
	return s.repo.GetByID(ctx, userID, uploadID)
}
