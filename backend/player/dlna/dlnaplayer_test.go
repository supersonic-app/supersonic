package dlna

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestHandleRequestUsesMusicServerClient(t *testing.T) {
	var requestedURL string
	musicServerClient := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		requestedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader("stream")),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
	player := &DLNAPlayer{musicServerClient: musicServerClient}
	key := player.addURLToProxy("http://music.example.test/stream")

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "http://renderer.local/"+key, nil)
	player.handleRequest(response, request)

	if requestedURL != "http://music.example.test/stream" {
		t.Fatalf("forwarded URL = %q, want music server URL", requestedURL)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Body.String() != "stream" {
		t.Fatalf("response body = %q, want %q", response.Body.String(), "stream")
	}
}

func TestRendererRetryClientDoesNotProxyLocalControl(t *testing.T) {
	retry := newRendererRetryClient()
	transport, ok := retry.HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("renderer transport = %T, want *http.Transport", retry.HTTPClient.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("renderer control client has a proxy configured")
	}
}
