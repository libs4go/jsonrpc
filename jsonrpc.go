package jsonrpc

import "fmt"

// RPCRequest represents a jsonrpc request object.
//
// See: http://www.jsonrpc.org/specification#request_object
type RPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
	ID      uint        `json:"id"`
}

// RPCNotification represents a jsonrpc notification object.
// A notification object omits the id field since there will be no server response.
//
// See: http://www.jsonrpc.org/specification#notification
type RPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// RPCResponse represents a jsonrpc response object.
// If no rpc specific error occurred Error field is nil.
//
// See: http://www.jsonrpc.org/specification#response_object
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
	ID      uint        `json:"id"`
}

// BatchResponse a list of jsonrpc response objects as a result of a batch request
//
// if you are interested in the response of a specific request use: GetResponseOf(request)
type BatchResponses []RPCResponse

// RPCError represents a jsonrpc error object if an rpc error occurred.
//
// See: http://www.jsonrpc.org/specification#error_object
type RPCError struct {
	Code    RPCErrorCode `json:"code"`
	Message string       `json:"message"`
	Data    interface{}  `json:"data"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPCError(%d) %s", e.Code, e.Message)
}

// RPCErrorCode represents jsonrpc error code
type RPCErrorCode int

const (
	RPCParseError     RPCErrorCode = -32700
	RPCInvalidRequest RPCErrorCode = -32600
	RPCMethodNotFound RPCErrorCode = -32601
	RPCInvalidParams  RPCErrorCode = -32602
	RPCInternalError  RPCErrorCode = -32603
	RPCServerError    RPCErrorCode = -32000
)
