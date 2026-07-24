package webhook

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/claudiovaldi/order-mock-payment/internal/httpresp"
	"github.com/claudiovaldi/order-mock-payment/internal/payment"
)

const SignatureHeader = "X-Signature"

const (
	CodeInvalidSignature        = "INVALID_SIGNATURE"
	CodeInvalidStatus           = "INVALID_STATUS"
	CodePaymentNotFound         = "PAYMENT_NOT_FOUND"
	CodeInvalidStatusTransition = "INVALID_STATUS_TRANSITION"
)

type webhookService interface {
	Process(ctx context.Context, payload []byte, signature string) error
}

type Handler struct {
	svc webhookService
}

func NewHandler(svc webhookService) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Callback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpresp.Error(c, http.StatusBadRequest, httpresp.CodeInvalidRequest, "cannot read request body")
		return
	}
	if err := h.svc.Process(c.Request.Context(), body, c.GetHeader(SignatureHeader)); err != nil {
		status, code, message := mapDomainError(err)
		httpresp.Error(c, status, code, message)
		return
	}
	httpresp.Success(c, http.StatusOK, gin.H{})
}

func mapDomainError(err error) (int, string, string) {
	switch {
	case errors.Is(err, ErrInvalidSignature):
		return http.StatusUnauthorized, CodeInvalidSignature, "invalid signature"
	case errors.Is(err, ErrInvalidRequest):
		return http.StatusBadRequest, httpresp.CodeInvalidRequest, "malformed request"
	case errors.Is(err, ErrInvalidStatus):
		return http.StatusBadRequest, CodeInvalidStatus, err.Error()
	case errors.Is(err, payment.ErrPaymentNotFound):
		return http.StatusNotFound, CodePaymentNotFound, "payment not found"
	case errors.Is(err, payment.ErrInvalidStatusTransition):
		return http.StatusConflict, CodeInvalidStatusTransition, err.Error()
	default:
		return http.StatusInternalServerError, httpresp.CodeInternalError, "internal server error"
	}
}
