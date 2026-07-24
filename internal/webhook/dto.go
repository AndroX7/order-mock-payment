package webhook

type CallbackRequest struct {
	ProviderReference string `json:"provider_reference"`
	Status            string `json:"status"`
}
