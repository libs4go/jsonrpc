package transport

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"github.com/libs4go/errors"
	"github.com/libs4go/jsonrpc"
	"github.com/libs4go/slf4go"
)

type WebSocketServer struct {
	slf4go.Logger
	jsonrpc.Server
}

var upgrader = websocket.Upgrader{}

func ServeWebSocket(server jsonrpc.Server) *WebSocketServer {
	return &WebSocketServer{
		Logger: slf4go.Get("JSONRPC-TRANSPORT-WEBSOCKET-SERVER"),
		Server: server,
	}
}

func (server *WebSocketServer) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	c, err := upgrader.Upgrade(writer, req, nil)

	if err != nil {
		server.E("upgrader error {@err}", err)
		return
	}

	defer c.Close()

	for {
		mt, message, err := c.ReadMessage()

		if err != nil {
			server.E("read error {@err}", err)
			break
		}

		if mt != websocket.TextMessage {
			continue
		}

		go func() {
			respBuff, err := server.Dispatch(context.Background(), message)

			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				server.E("server internal error %s", err.Error())
				return
			}

			if len(respBuff) != 0 {

				err = c.WriteMessage(websocket.TextMessage, respBuff)

				if err != nil {
					server.E("server resp write error %s", err.Error())
				}
			}
		}()

	}
}

// WebSocket client transport
type websocketClientTransport struct {
	slf4go.Logger
	u             *url.URL
	recv          chan []byte
	client        *websocket.Conn
	customHeaders map[string][]string
}

type WebSocketOps func(*websocketClientTransport)

func WebSocketHeaders(headers map[string][]string) WebSocketOps {
	return func(hct *websocketClientTransport) {
		hct.customHeaders = headers
	}
}

func NewWebSocketClientTransport(serviceURL string, ops ...WebSocketOps) (jsonrpc.ClientTransport, error) {
	u, err := url.Parse(serviceURL)

	if err != nil {
		return nil, errors.Wrap(err, "parse server url error, %s", serviceURL)
	}

	transport := &websocketClientTransport{
		Logger:        slf4go.Get("JSONRPC-TRANSPORT-WEBSOCKET-CLIENT"),
		u:             u,
		recv:          make(chan []byte, 100),
		customHeaders: make(map[string][]string),
	}

	for _, op := range ops {
		op(transport)
	}

	c, _, err := websocket.DefaultDialer.Dial(serviceURL, http.Header(transport.customHeaders))

	if err != nil {
		return nil, err
	}

	transport.client = c

	go transport.runLoop()

	return transport, nil
}

func (transport *websocketClientTransport) runLoop() {
	defer close(transport.recv)

	for {
		mt, message, err := transport.client.ReadMessage()

		if err != nil {
			transport.E("recv message error {@err}", err)
			return
		}

		if mt != websocket.TextMessage {
			transport.W("skip recv message({@t}) {@msg}", mt, message)
			continue
		}

		transport.recv <- message
	}
}

func (transport *websocketClientTransport) Close() {
	transport.client.Close()
}

func (transport *websocketClientTransport) Send(ctx context.Context, body []byte) error {

	err := transport.client.WriteMessage(websocket.TextMessage, body)

	if err != nil {
		return errors.Wrap(err, "send message error")
	}

	return nil
}

func (transport *websocketClientTransport) Recv() <-chan []byte {
	return transport.recv
}
