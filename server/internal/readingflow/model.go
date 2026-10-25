package readingflow

import (
	"context"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// ModelSpec is the vendor binding the admin stored. Eino does not choose the vendor.
type ModelSpec struct {
	Vendor    string
	Model     string
	BaseURL   string
	APIKey    string
	Timeout   time.Duration
	Transport http.RoundTripper
}

func newChatModel(ctx context.Context, spec ModelSpec) (model.BaseChatModel, error) {
	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	transport := spec.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  spec.APIKey,
		Model:   spec.Model,
		BaseURL: spec.BaseURL,
		Timeout: timeout,
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	})
}
