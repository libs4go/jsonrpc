package jsonrpc

import "github.com/libs4go/errors"

// ScopeOfAPIError .
const errVendor = "jsonrpc"

// errors
var (
	ErrTimeout    = errors.New("RPC timeout", errors.WithVendor(errVendor), errors.WithCode(-1))
	ErrClose      = errors.New("RPC endpoint closed", errors.WithVendor(errVendor), errors.WithCode(-2))
	ErrTransport  = errors.New("expect transport", errors.WithVendor(errVendor), errors.WithCode(-3))
	ErrDispatcher = errors.New("expect dispatcher", errors.WithVendor(errVendor), errors.WithCode(-4))
	ErrServer     = errors.New("Server type error", errors.WithVendor(errVendor), errors.WithCode(-5))
)
