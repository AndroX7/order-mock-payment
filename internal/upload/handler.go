package upload

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
	"github.com/claudiovaldi/order-mock-payment/internal/middleware"
)

const (
	CodeInvalidMultipart = "INVALID_MULTIPART"
	CodeMissingFile      = "MISSING_FILE"
	CodeOrderNotFound    = "ORDER_NOT_FOUND"
	CodeEmptyFile        = "EMPTY_FILE"
	CodeFileTooLarge     = "FILE_TOO_LARGE"
	CodeUnsupportedType  = "UNSUPPORTED_CONTENT_TYPE"
	CodeUploadNotFound   = "UPLOAD_NOT_FOUND"
)

type UploadService interface {
	Create(ctx context.Context, userID, orderID uuid.UUID, file multipart.File, size int64) (*Upload, error)
	Get(ctx context.Context, userID, uploadID uuid.UUID) (*Upload, error)
}

type Handler struct {
	svc UploadService
}

func NewHandler(svc UploadService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Create(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}

	orderID, err := uuid.Parse(c.PostForm("order_id"))
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "invalid or missing order_id")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			httpresp.Error(c, http.StatusBadRequest, CodeMissingFile, "file field is required")
			return
		}
		httpresp.Error(c, http.StatusBadRequest, CodeInvalidMultipart, "invalid multipart form")
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		httpresp.Error(c, http.StatusInternalServerError, httpresp.CodeInternalError, "cannot open uploaded file")
		return
	}
	defer func() { _ = file.Close() }()

	u, err := h.svc.Create(c.Request.Context(), userID, orderID, file, fileHeader.Size)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusCreated, gin.H{"upload": NewUploadResponse(u)})
}

func (h *Handler) Get(c *gin.Context) {
	userID, ok := middleware.RequireUserID(c)
	if !ok {
		return
	}
	uploadID, ok := middleware.ParseIDParam(c, "upload")
	if !ok {
		return
	}
	u, err := h.svc.Get(c.Request.Context(), userID, uploadID)
	if err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusOK, gin.H{"upload": NewUploadResponse(u)})
}

func mapDomainError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrOrderNotFound):
		return http.StatusNotFound, CodeOrderNotFound, err.Error()
	case errors.Is(err, ErrEmptyFile):
		return http.StatusBadRequest, CodeEmptyFile, err.Error()
	case errors.Is(err, ErrFileTooLarge):
		return http.StatusRequestEntityTooLarge, CodeFileTooLarge, err.Error()
	case errors.Is(err, ErrUnsupportedContentType):
		return http.StatusUnsupportedMediaType, CodeUnsupportedType, err.Error()
	case errors.Is(err, ErrUploadNotFound):
		return http.StatusNotFound, CodeUploadNotFound, err.Error()
	default:
		return http.StatusInternalServerError, httpresp.CodeInternalError, "internal server error"
	}
}
