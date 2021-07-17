package jsonrpc

import (
	"context"
	"reflect"
	"sync"
)

type callSite struct {
	name   string
	server reflect.Value
	method reflect.Value
}

func newCallSite(server interface{}, name string) *callSite {
	serverValue := reflect.ValueOf(server)
	method := serverValue.MethodByName(name)

	return &callSite{
		name:   name,
		server: serverValue,
		method: method,
	}
}

func (cs *callSite) Call(ctx context.Context, req *RPCRequest, resp ResponseWriter) {
	if cs.method.IsZero() {
		resp.Error(RPCInvalidRequest, "invalid rpc call %s", cs.name)
		return
	}

}

func (cs *callSite) Notification(ctx context.Context, req *RPCNotification) {

}

type reflectDispatcher struct {
	sync.RWMutex
	server    interface{}
	callSites map[string]*callSite
}

func Dispatch(server interface{}) ServerOpt {
	return func(server *Server) {
		server.Dispatcher = &reflectDispatcher{
			server:    server,
			callSites: make(map[string]*callSite),
		}
	}
}

func (dispatcher *reflectDispatcher) Call(ctx context.Context, req *RPCRequest, resp ResponseWriter) {
	dispatcher.callSite(req.Method).Call(ctx, req, resp)
}

func (dispatcher *reflectDispatcher) callSite(name string) *callSite {
	dispatcher.RLock()
	cs, ok := dispatcher.callSites[name]
	dispatcher.RUnlock()

	if !ok {
		dispatcher.Lock()
		cs, ok = dispatcher.callSites[name]

		if !ok {
			cs = newCallSite(dispatcher.server, name)
			dispatcher.callSites[name] = cs
		}

		dispatcher.Unlock()
	}

	return cs
}

func (dispatcher *reflectDispatcher) Notification(ctx context.Context, req *RPCNotification) {
	dispatcher.callSite(req.Method).Notification(ctx, req)
}
