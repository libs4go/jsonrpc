package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/libs4go/errors"
	"github.com/libs4go/slf4go"
)

type ServerRequest struct {
	Request        []byte
	ResponseWriter func([]byte) error
}

type ServerTransport interface {
	Recv() <-chan *ServerRequest
	Close() error
}

// Server server object
type Server struct {
	slf4go.Logger
	Transport  ServerTransport
	Dispatcher Dispatcher
	ctx        context.Context
	cancelF    context.CancelFunc
	timeout    time.Duration
}

// ServerOpt .
type ServerOpt func(server *Server)

// ServerContext set server context
func ServerContext(ctx context.Context) ServerOpt {
	return func(server *Server) {
		server.ctx = ctx
	}
}

// ServerTransport set server transport
func ServerTrans(transport ServerTransport) ServerOpt {
	return func(server *Server) {
		server.Transport = transport
	}
}

type ResponseWriter interface {
	Error(code RPCErrorCode, format string, args ...interface{})
	Result(value interface{})
}

type responseWriter struct {
	err    *RPCError
	result interface{}
}

func (writer *responseWriter) Error(code RPCErrorCode, format string, args ...interface{}) {
	writer.err = &RPCError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

func (writer *responseWriter) Result(value interface{}) {
	writer.result = value
}

func (writer *responseWriter) generate(id uint) *RPCResponse {
	resp := &RPCResponse{
		ID:     id,
		Result: writer.result,
	}

	if writer.err != nil {
		resp.Error = writer.err
	}

	return resp
}

func serverNullCheck(server *Server) error {
	if server.Transport == nil {
		return errors.Wrap(ErrTransport, "expect transport ops")
	}

	if server.Dispatcher == nil {
		return errors.Wrap(ErrDispatcher, "expect dispatcher ops")
	}

	if server.ctx == nil {
		server.ctx = context.Background()
	}

	return nil
}

func newServer(opts ...ServerOpt) (*Server, error) {

	server := &Server{
		Logger: slf4go.Get("jsonrpc"),
	}

	for _, opt := range opts {
		opt(server)
	}

	if err := serverNullCheck(server); err != nil {
		return nil, err
	}

	newCtx, cancelF := context.WithCancel(server.ctx)

	server.ctx = newCtx
	server.cancelF = cancelF

	go server.runLoop()

	return server, nil
}

func (server *Server) runLoop() {
	for {
		select {
		case <-server.ctx.Done():
			return
		case req, ok := <-server.Transport.Recv():
			if !ok {
				return
			}

			if len(req.Request) == 0 {
				continue
			}

			var request map[string]interface{}

			err := json.Unmarshal(req.Request, &request)

			if err != nil {
				server.E("unmarshal resp {@buff} err {@err}", req.Request, err)
				continue
			}

			go server.dispatch(req.ResponseWriter, request)
		}
	}
}

func (server *Server) dispatch(respWriter func([]byte) error, request map[string]interface{}) {

	ctx, cancel := context.WithTimeout(server.ctx, server.timeout)

	defer cancel()

	if _, ok := request["id"]; ok {

		buff, err := json.Marshal(request)

		if err != nil {
			server.E("marshal req {@req} err {@err}", request, err)
			return
		}

		var req *RPCRequest

		err = json.Unmarshal(buff, &req)

		if err != nil {
			server.E("unmarshal req {@req} err {@err}", request, err)
			return
		}

		writer := &responseWriter{}

		server.Dispatcher.Call(ctx, req, writer)

		resp := writer.generate(req.ID)

		buff, err = json.Marshal(resp)

		if err != nil {
			server.E("marshal resp {@resp} err {@err}", resp, err)
			return
		}

		if err := respWriter(buff); err != nil {
			server.E("send resp {@resp} err {@err}", resp, err)
			return
		}

		return
	}

	buff, err := json.Marshal(request)

	if err != nil {
		server.E("marshal notification {@req} err {@err}", request, err)
		return
	}

	var notification *RPCNotification

	err = json.Unmarshal(buff, &notification)

	if err != nil {
		server.E("unmarshal notification {@notification} err {@err}", request, err)
		return
	}

	server.Dispatcher.Notification(ctx, notification)
}

func (server *Server) Close() error {
	server.cancelF()
	return nil
}

func NewHTPPServer(opts ...ServerOpt) (*Server, http.Handler, error) {
	transport, err := NewHTTPServerTransport()

	if err != nil {
		return nil, nil, err
	}

	server, err := newServer(append(opts, ServerTrans(transport))...)

	if err != nil {
		return nil, nil, err
	}

	return server, transport, nil
}
