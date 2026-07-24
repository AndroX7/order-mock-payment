package upload

import (
	"time"

	"github.com/google/uuid"
)

// UploadResponse is the client-facing view of an Upload.
type UploadResponse struct {
	ID          uuid.UUID `json:"id"`
	OrderID     uuid.UUID `json:"order_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewUploadResponse(u *Upload) UploadResponse {
	return UploadResponse{
		ID:          u.ID,
		OrderID:     u.OrderID,
		Filename:    u.Filename,
		ContentType: u.ContentType,
		Size:        u.Size,
		Path:        u.Path,
		CreatedAt:   u.CreatedAt,
	}
}
