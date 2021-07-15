package jsonrpc

import "context"

type reflectDispatcher struct {
	server interface{}
}

func Dispatch(server interface{}) ServerOpt {
	return func(server *Server) {
		server.Dispatcher = &reflectDispatcher{
			server: server,
		}
	}
}

func (dispatcher *reflectDispatcher) Call(ctx context.Context, req *RPCRequest, resp ResponseWriter) {

}

func (dispatcher *reflectDispatcher) Notification(ctx context.Context, req *RPCNotification) {

}
