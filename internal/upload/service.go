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

type OrderService interface {
	Get(ctx context.Context, userID, orderID uuid.UUID) (*order.Order, error)
}

var allowedMIMEs = map[string]string{
	"application/pdf": "pdf",
	"image/png":       "png",
	"image/jpeg":      "jpg",
}

type Service struct {
	repo    Repository
	storage Storage
	orders  OrderService
	maxSize int64
}

func NewService(repo Repository, storage Storage, orders OrderService, maxSize int64) *Service {
	return &Service{repo: repo, storage: storage, orders: orders, maxSize: maxSize}
}

func (s *Service) Create(ctx context.Context, userID, orderID uuid.UUID, file multipart.File, size int64) (*Upload, error) {
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

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek file: %w", err)
	}

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

func (s *Service) Get(ctx context.Context, userID, uploadID uuid.UUID) (*Upload, error) {
	return s.repo.GetByID(ctx, userID, uploadID)
}
