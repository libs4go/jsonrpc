package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sync"

	"github.com/libs4go/errors"
	"github.com/libs4go/jsonrpc"
	"github.com/libs4go/jsonrpc/transport"
	"github.com/libs4go/slf4go"
)

func unmarshalArray(data []byte, types []reflect.Type) ([]reflect.Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	var args []reflect.Value
	tok, err := dec.Token()
	switch {
	case err == io.EOF || tok == nil && err == nil:
		// "params" is optional and may be empty. Also allow "params":null even though it's
		// not in the spec because our own client used to send it.
	case err != nil:
		return nil, err
	case tok == json.Delim('['):
		// Read argument array.
		if args, err = parseArgumentArray(dec, types); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("non-array args")
	}
	// Set any missing args to nil.
	for i := len(args); i < len(types); i++ {
		if types[i].Kind() != reflect.Ptr {
			return nil, fmt.Errorf("missing value for required argument %d", i)
		}
		args = append(args, reflect.Zero(types[i]))
	}
	return args, nil
}

func parseArgumentArray(dec *json.Decoder, types []reflect.Type) ([]reflect.Value, error) {
	args := make([]reflect.Value, 0, len(types))
	for i := 0; dec.More(); i++ {
		if i >= len(types) {
			return args, fmt.Errorf("too many arguments, want at most %d", len(types))
		}
		argval := reflect.New(types[i])
		if err := dec.Decode(argval.Interface()); err != nil {
			return args, fmt.Errorf("invalid argument %d: %v", i, err)
		}
		if argval.IsNil() && types[i].Kind() != reflect.Ptr {
			return args, fmt.Errorf("missing value for required argument %d", i)
		}
		args = append(args, argval.Elem())
	}
	// Read end of args array.
	_, err := dec.Token()
	return args, err
}

type callSite struct {
	in     []reflect.Type
	out    []reflect.Type
	method reflect.Method
}

func (cs *callSite) Call(ctx context.Context, server *serverImpl, writer *responseWriter, rpcRequest *jsonrpc.RPCRequest) {

	buff, err := json.Marshal(rpcRequest.Params)

	if err != nil {
		writer.Error(jsonrpc.RPCInvalidRequest, "marshal params error %s", err.Error())
		return
	}

	params, err := unmarshalArray(buff, cs.in)

	for i := len(params); i < len(cs.in); i++ {

		parmType := cs.in[i]

		if parmType.Kind() != reflect.Ptr {
			writer.Error(jsonrpc.RPCInvalidParams, "missing value for required argument %d", i)
			return
		}

		params = append(params, reflect.Zero(parmType))
	}

	params = append([]reflect.Value{server.server}, params...)

	returns := cs.method.Func.Call(params)

	errValue := returns[len(returns)-1].Interface()

	if rpcRequest.ID == nil {
		if errValue != nil {
			server.E("call notification method {@name} error {@err}", cs.method.Name, err)
		}

		return
	}

	if errValue != nil {
		writer.Error(jsonrpc.RPCInternalError, "%s", errValue.(error).Error())
		return
	}

	if len(returns) > 2 {
		var arr []interface{}

		for i := 0; i < len(returns)-1; i++ {
			arr = append(arr, returns[i].Interface())
		}

		writer.Result(arr)
	} else {
		writer.Result(returns[0].Interface())
	}

}

type responseWriter struct {
	err    *jsonrpc.RPCError
	result interface{}
}

func (writer *responseWriter) Error(code jsonrpc.RPCErrorCode, format string, args ...interface{}) {
	writer.err = &jsonrpc.RPCError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

func (writer *responseWriter) Result(value interface{}) {
	writer.result = value
}

func (writer *responseWriter) marshal(id uint) ([]byte, error) {
	resp := &jsonrpc.RPCResponse{
		ID:      id,
		Result:  writer.result,
		JSONRPC: "2.0",
	}

	if writer.err != nil {
		resp.Error = writer.err
	}

	buff, err := json.Marshal(resp)

	if err != nil {
		return nil, errors.Wrap(err, "marshal resp error")
	}

	return buff, nil
}

type serverImpl struct {
	sync.RWMutex
	slf4go.Logger
	methods map[string]*callSite
	server  reflect.Value
}

func New(server interface{}) (jsonrpc.Server, error) {
	s := &serverImpl{
		Logger:  slf4go.Get("JSONRPC-SERVER"),
		methods: make(map[string]*callSite),
	}

	err := s.reflectCreateServer(server)

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (server *serverImpl) reflectCreateServer(wrapped interface{}) error {
	serverType := reflect.TypeOf(wrapped)

	serverValue := reflect.ValueOf(wrapped)

	if serverType.Kind() != reflect.Ptr {
		return errors.Wrap(jsonrpc.ErrServer, "server type must be struct ptr")
	}

	if serverType.Elem().Kind() != reflect.Struct {
		return errors.Wrap(jsonrpc.ErrServer, "server type must be struct ptr")
	}

	errorInterface := reflect.TypeOf((*error)(nil)).Elem()

	for i := 0; i < serverType.NumMethod(); i++ {

		methodType := serverType.Method(i)

		server.I("reflect server method {@name}", methodType.Name)

		if methodType.Type.NumOut() < 1 {
			server.W("skip method {@name}, last out param must be error", methodType.Name)
			continue
		}

		lastOutType := methodType.Type.Out(methodType.Type.NumOut() - 1)

		if !lastOutType.Implements(errorInterface) {
			server.W("skip method {@name}, last out param must be error", methodType.Name)
			continue
		}

		var inTypes []reflect.Type

		for i := 1; i < methodType.Type.NumIn(); i++ {
			inTypes = append(inTypes, methodType.Type.In(i))
		}

		var outTypes []reflect.Type

		for i := 0; i < methodType.Type.NumOut()-1; i++ {
			outTypes = append(outTypes, methodType.Type.Out(i))
		}

		server.methods[methodType.Name] = &callSite{
			in:     inTypes,
			out:    outTypes,
			method: methodType,
		}

		server.I("reflect server method {@name} -- success", methodType.Name)
	}

	server.server = serverValue

	return nil
}

func (server *serverImpl) Dispatch(ctx context.Context, buff []byte) ([]byte, error) {
	var rpcRequest *jsonrpc.RPCRequest
	err := json.Unmarshal(buff, &rpcRequest)

	if err != nil {
		return nil, errors.Wrap(err, "unmarshal request error")
	}

	server.RLock()
	defer server.RUnlock()

	writer := &responseWriter{}

	cs, ok := server.methods[rpcRequest.Method]

	if !ok {
		if rpcRequest.ID != nil {
			writer.Error(jsonrpc.RPCInvalidRequest, "unspport method %s", rpcRequest.Method)
			return writer.marshal(*rpcRequest.ID)
		} else {
			return nil, errors.Wrap(jsonrpc.ErrDispatcher, "unspport method %s", rpcRequest.Method)
		}
	} else {
		cs.Call(ctx, server, writer, rpcRequest)

		if rpcRequest.ID != nil {
			return writer.marshal(*rpcRequest.ID)
		}

		return nil, nil
	}

	return nil, nil
}

// NewHTTPServer create http server
func NewHTTPServer(server interface{}) (*transport.HTTPServer, error) {
	s, err := New(server)

	if err != nil {
		return nil, err
	}

	return transport.ServeHTTP(s), nil
}
