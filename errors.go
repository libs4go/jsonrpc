package jsonrpc

import "github.com/libs4go/errors"

// ScopeOfAPIError .
const errVendor = "jsonrpc"

// errors
var (
	ErrTimeout = errors.New("RPC timeout", errors.WithVendor(errVendor), errors.WithCode(-1))
	ErrClose   = errors.New("RPC endpoint closed", errors.WithVendor(errVendor), errors.WithCode(-2))
)
