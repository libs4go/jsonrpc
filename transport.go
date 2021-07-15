package jsonrpc

import (
	"net/http"
	"net/url"

	"github.com/libs4go/errors"
)

type Transport interface {
	Send([]byte) error
	Recv() <-chan []byte
}

type TransportCloser interface {
	Transport
	Close() error
}

// HTTP client transport
type httpClientTransport struct {
	u *url.URL
}

func NewHTTPClientTransport(serviceURL string) (Transport, error) {
	u, err := url.Parse(serviceURL)

	if err != nil {
		return nil, errors.Wrap(err, "parse server url error, %s", serviceURL)
	}

	return &httpClientTransport{
		u: u,
	}, nil
}

func (transport *httpClientTransport) Send([]byte) error {
	return nil
}

func (transport *httpClientTransport) Recv() <-chan []byte {
	return nil
}

// HTTP server transport
type httpServerTransport struct {
}

type HTTPServerTransport interface {
	Transport
	http.Handler
}

func NewHTTPServerTransport() (HTTPServerTransport, error) {
	return &httpServerTransport{}, nil
}

func (transport *httpServerTransport) Send([]byte) error {
	return nil
}

func (transport *httpServerTransport) Recv() <-chan []byte {
	return nil
}

func (transport *httpServerTransport) ServeHTTP(http.ResponseWriter, *http.Request) {

}
