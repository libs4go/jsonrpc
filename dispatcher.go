package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/libs4go/errors"
)

type callSite struct {
	name       string
	server     reflect.Value
	method     reflect.Value
	methodType reflect.Type
}

func newCallSite(server interface{}, name string) *callSite {
	serverValue := reflect.ValueOf(server)
	method := serverValue.MethodByName(name)

	methodType := method.Type()

	if methodType.NumOut() < 1 {
		method = reflect.Value{}
	} else {
		errorInterface := reflect.TypeOf((*error)(nil)).Elem()
		if !methodType.Out(methodType.NumOut() - 1).Implements(errorInterface) {
			method = reflect.Value{}
		}
	}

	return &callSite{
		name:       name,
		server:     serverValue,
		method:     method,
		methodType: methodType,
	}
}

func (cs *callSite) Call(ctx context.Context, req *RPCRequest, resp ResponseWriter) {
	if cs.method.IsZero() {
		resp.Error(RPCInvalidRequest, "invalid rpc call %s", cs.name)
		return
	}

	if req.Method != cs.name {
		panic("call invalid callsite")
	}

	if cs.method.Type().NumOut() < 2 {
		resp.Error(RPCInvalidRequest, "invalid rpc call %s", cs.name)
		return
	}

	params, err := cs.getParams(req.Params)

	if err != nil {
		resp.Error(RPCInvalidParams, "%s", err.Error())
		return
	}

	results := cs.method.Call(params)

	cs.writeResponse(resp, results)
}

func (cs *callSite) writeResponse(resp ResponseWriter, results []reflect.Value) {

	if len(results) < 2 {
		panic("require out number >= 2")
	}

	errValue := results[len(results)-1]

	if !errValue.IsZero() {
		resp.Error(RPCServerError, "%s", errValue.Interface().(error).Error())
		return
	}

	if len(results) == 2 {
		resp.Result(results[0].Interface())
		return
	}

	var arr []interface{}

	for i := 0; i < len(results)-1; i++ {
		arr = append(arr, results[i].Interface())
	}

	resp.Result(arr)
}

func (cs *callSite) getParams(params interface{}) ([]reflect.Value, error) {

	buff, err := json.Marshal(params)

	if err != nil {
		return nil, errors.Wrap(err, "marshal params error")
	}

	paramsType := reflect.TypeOf(params)

	var types []reflect.Type

	for i := 0; i < paramsType.NumIn(); i++ {
		types = append(types, paramsType.In(i))
	}

	results, err := unmarshalArray(buff, types)

	if err != nil {
		return nil, err
	}

	for i := len(results); i < paramsType.NumIn(); i++ {

		parmType := paramsType.In(i)

		if parmType.Kind() != reflect.Ptr {
			return nil, fmt.Errorf("missing value for required argument %d", i)
		}

		results = append(results, reflect.Zero(parmType))
	}

	return results, nil
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
