package jsonrpc

import (
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

type httpClientTransport struct {
	u *url.URL
}

func NewHTTPClient(serviceURL string) (Transport, error) {
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
