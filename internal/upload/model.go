package upload

import (
	"time"

	"github.com/google/uuid"
)

// Upload is the domain type persisted in the uploads table.
// Uploads are immutable: no updated_at, no update flow.
type Upload struct {
	ID          uuid.UUID `db:"id"`
	OrderID     uuid.UUID `db:"order_id"`
	Filename    string    `db:"filename"`
	ContentType string    `db:"content_type"`
	Size        int64     `db:"size"`
	Path        string    `db:"path"`
	CreatedAt   time.Time `db:"created_at"`
}
