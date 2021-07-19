package jsonrpc

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/libs4go/errors"
)

// HTTP client transport
type httpClientTransport struct {
	u             *url.URL
	recv          chan []byte
	client        *http.Client
	customHeaders map[string]string
}

type HTTPClientOps func(*httpClientTransport)

func RequestHeaders(headers map[string]string) HTTPClientOps {
	return func(hct *httpClientTransport) {
		hct.customHeaders = headers
	}
}

func NewHTTPClientTransport(serviceURL string, ops ...HTTPClientOps) (ClientTransport, error) {
	u, err := url.Parse(serviceURL)

	if err != nil {
		return nil, errors.Wrap(err, "parse server url error, %s", serviceURL)
	}

	transport := &httpClientTransport{
		u:    u,
		recv: make(chan []byte, 100),
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 40,
				IdleConnTimeout:     time.Duration(20) * time.Second,
			},

			Timeout: 20 * time.Second,
		},
		customHeaders: make(map[string]string),
	}

	for _, op := range ops {
		op(transport)
	}

	return transport, nil
}

func (transport *httpClientTransport) Close() {
	close(transport.recv)
}

func (transport *httpClientTransport) Send(body []byte) (err error) {
	request, err := http.NewRequest("POST", transport.u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "create post request error")
	}

	for k, v := range transport.customHeaders {
		request.Header.Add(k, v)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	transport.client.Do(request)

	httpResponse, err := transport.client.Do(request)

	if err != nil {
		return err
	}

	defer httpResponse.Body.Close()

	buff, err := io.ReadAll(httpResponse.Body)

	if err != nil {
		return errors.Wrap(err, "read http resp body error")
	}

	defer func() {
		if e := recover(); err != nil {
			err = e.(error)
		}
	}()

	transport.recv <- buff

	return nil
}

func (transport *httpClientTransport) Recv() <-chan []byte {
	return transport.recv
}

type HTTPServerTransport interface {
	ServerTransport
	http.Handler
}

// HTTP server transport
type httpServerTransport struct {
	recv chan *ServerRequest
}

type HTTPServerTransportOp func(*httpServerTransport)

func NewHTTPServerTransport(ops ...HTTPServerTransportOp) (HTTPServerTransport, error) {
	t := &httpServerTransport{}

	for _, op := range ops {
		op(t)
	}

	if t.recv == nil {
		t.recv = make(chan *ServerRequest, 200)
	}

	return t, nil
}

func (transport *httpServerTransport) Close() error {
	close(transport.recv)

	return nil
}

func (transport *httpServerTransport) Recv() <-chan *ServerRequest {
	return transport.recv
}

func (transport *httpServerTransport) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	defer func() {
		if e := recover(); e != nil {
			writer.WriteHeader(http.StatusServiceUnavailable)
			writer.Write([]byte("server closed"))
		}
	}()

	defer req.Body.Close()

	buff, err := io.ReadAll(req.Body)

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	transport.recv <- &ServerRequest{
		Request: buff,
		ResponseWriter: func(b []byte) error {
			_, err := writer.Write(b)

			return err
		},
	}
}
