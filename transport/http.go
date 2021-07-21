package transport

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/libs4go/errors"
	"github.com/libs4go/jsonrpc"
	"github.com/libs4go/slf4go"
)

type HTTPServer struct {
	slf4go.Logger
	jsonrpc.Server
}

func ServeHTTP(server jsonrpc.Server) *HTTPServer {
	return &HTTPServer{
		Logger: slf4go.Get("JSONRPC-TRANSPORT-HTTP-SERVER"),
		Server: server,
	}
}

func (server *HTTPServer) ServeHTTP(writer http.ResponseWriter, resq *http.Request) {
	defer resq.Body.Close()

	buff, err := io.ReadAll(resq.Body)

	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		writer.Write([]byte("read http body error"))
		return
	}

	respBuff, err := server.Dispatch(context.Background(), buff)

	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		server.E("server internal error {@err}", err.Error())
		return
	}

	if len(respBuff) != 0 {
		_, err = writer.Write(respBuff)

		if err != nil {
			server.E("server resp write error {@err}", err.Error())
		}
	}
}

// HTTP client transport
type httpClientTransport struct {
	u             *url.URL
	recv          chan []byte
	client        *http.Client
	customHeaders map[string]string
}

type HTTPClientOps func(*httpClientTransport)

func HTTPHeaders(headers map[string]string) HTTPClientOps {
	return func(hct *httpClientTransport) {
		hct.customHeaders = headers
	}
}

func NewHTTPClientTransport(serviceURL string, ops ...HTTPClientOps) (jsonrpc.ClientTransport, error) {
	u, err := url.Parse(serviceURL)

	if err != nil {
		return nil, errors.Wrap(err, "parse server url error, %s", serviceURL)
	}

	transport := &httpClientTransport{
		u:             u,
		recv:          make(chan []byte, 100),
		client:        http.DefaultClient,
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

func (transport *httpClientTransport) Send(ctx context.Context, body []byte) (err error) {

	request, err := http.NewRequest("POST", transport.u.String(), bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "create post request error")
	}

	for k, v := range transport.customHeaders {
		request.Header.Add(k, v)
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")

	httpResponse, err := transport.client.Do(request)

	if err != nil {
		println(err.Error())
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
