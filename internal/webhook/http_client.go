package webhook

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shyxur/flowforge/internal/ports"
)

const DefaultResponseBodyLimit = 4 * 1024

type HTTPClient struct {
	client            *http.Client
	responseBodyLimit int64
}

func NewHTTPClient(timeout time.Duration, responseBodyLimit int64) *HTTPClient {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if responseBodyLimit <= 0 {
		responseBodyLimit = DefaultResponseBodyLimit
	}
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		responseBodyLimit: responseBodyLimit,
	}
}

func (client *HTTPClient) Send(ctx context.Context, request ports.WebhookHTTPRequest) (*ports.WebhookHTTPResponse, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	for name, value := range request.Headers {
		httpRequest.Header.Set(name, value)
	}
	response, err := client.client.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, client.responseBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > client.responseBodyLimit {
		body = body[:client.responseBodyLimit]
	}
	return &ports.WebhookHTTPResponse{
		StatusCode: response.StatusCode,
		Body:       strings.ToValidUTF8(string(body), "\uFFFD"),
	}, nil
}
