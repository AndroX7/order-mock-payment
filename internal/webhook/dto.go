package webhook

// CallbackRequest is the JSON payload providers send to POST /webhooks/payment.
//
// Per security policy the service only trusts provider_reference and the
// HMAC signature. Any amount/currency/user_id fields the provider might
// also include are deliberately not part of this struct — they are
// looked up server-side from the referenced payment.
type CallbackRequest struct {
	ProviderReference string `json:"provider_reference"`
	Status            string `json:"status"`
}
