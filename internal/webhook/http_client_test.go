package webhook

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/shyxur/flowforge/internal/ports"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestHTTPClientTruncatesResponseBody(t *testing.T) {
	client := NewHTTPClient(time.Second, 4096)
	client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 5000))),
			Header:     make(http.Header),
		}, nil
	})
	response, err := client.Send(context.Background(), ports.WebhookHTTPRequest{
		URL: "https://example.com/hook", Body: []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusInternalServerError || len(response.Body) != 4096 {
		t.Fatalf("status=%d body length=%d", response.StatusCode, len(response.Body))
	}
}
