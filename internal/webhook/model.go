package webhook

// Wire-format status values providers may send in a callback. Kept
// separate from payment.StatusPaid/StatusFailed because the payment
// package should not depend on the webhook wire format.
const (
	StatusPaid   = "paid"
	StatusFailed = "failed"
)
