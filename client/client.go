package client

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/libs4go/errors"
	"github.com/libs4go/jsonrpc"
	"github.com/libs4go/jsonrpc/transport"
	"github.com/libs4go/slf4go"
)

type Result struct {
	client *Client
	req    *jsonrpc.RPCRequest
	ctx    context.Context
}

func (result *Result) Cancel() {

}

func (result *Result) Join(resultObject interface{}) error {
	resp, err := result.client.send(result.ctx, result.req)

	if err != nil {
		return err
	}

	if resp.Error != nil {
		return resp.Error
	}

	js, err := json.Marshal(resp.Result)
	if err != nil {
		return errors.Wrap(err, "Marshal result error")
	}

	err = json.Unmarshal(js, resultObject)

	if err != nil {
		return errors.Wrap(err, "Unmarshal result error")
	}

	return nil
}

// Client jsonrpc client object
type Client struct {
	sync.Mutex
	slf4go.Logger
	Transport jsonrpc.ClientTransport            // Client transport
	seq       uint                               // request seq
	waitQ     map[uint]chan *jsonrpc.RPCResponse // waitQ
	timeout   time.Duration                      // rpc global timeout
	ctx       context.Context
	cancelF   context.CancelFunc
}

// ClientOpt .
type ClientOpt func(client *Client)

// ClientTransport set client transport
func ClientTrans(transport jsonrpc.ClientTransport) ClientOpt {
	return func(client *Client) {
		client.Transport = transport
	}
}

// ServerContext set server context
func ClientContext(ctx context.Context) ClientOpt {
	return func(client *Client) {
		client.ctx = ctx
	}
}

// ClientTimeout set client timeout
func ClientTimeout(duration time.Duration) ClientOpt {
	return func(client *Client) {
		client.timeout = duration
	}
}

func clientNullCheck(client *Client) error {
	if client.Transport == nil {
		return errors.Wrap(jsonrpc.ErrTransport, "expect transport ops")
	}

	if client.ctx == nil {
		client.ctx = context.Background()
	}

	return nil
}

func New(options ...ClientOpt) (jsonrpc.Client, error) {

	client := &Client{
		Logger:  slf4go.Get("JSONRPC-CLIENT"),
		waitQ:   make(map[uint]chan *jsonrpc.RPCResponse),
		timeout: time.Second * 60,
		seq:     1,
	}

	for _, opt := range options {
		opt(client)
	}

	if err := clientNullCheck(client); err != nil {
		return nil, err
	}

	newCtx, cancelF := context.WithCancel(client.ctx)

	client.ctx = newCtx
	client.cancelF = cancelF

	go client.runLoop()

	return client, nil
}

func (client *Client) runLoop() {
	for {
		select {
		case <-client.ctx.Done():
			client.clear()
			return
		case buff, ok := <-client.Transport.Recv():

			if !ok {
				client.clear()
				return
			}

			if len(buff) == 0 {
				continue
			}

			var resp *jsonrpc.RPCResponse

			err := json.Unmarshal(buff, &resp)

			if err != nil {
				client.E("unmarshal resp {@buff} err {@err}", buff, err)
				continue
			}

			client.D("recv remote message {@msg}", resp)

			result, ok := client.tryGetWait(resp.ID)

			if !ok {
				client.E("match request not found for {@resp}", resp)
				continue
			}

			client.sendResult(result, resp)
		}
	}

}

func (client *Client) sendResult(result chan *jsonrpc.RPCResponse, resp *jsonrpc.RPCResponse) {
	defer func() {
		if err := recover(); err != nil {
			client.E("send resp {@resp} error {@err}", resp, err)
		}
	}()

	result <- resp
}

func (client *Client) clear() {
	transportCloser, ok := client.Transport.(jsonrpc.ClientTransportCloser)

	if ok {
		transportCloser.Close()
	}
}

func (client *Client) Close() error {
	client.cancelF()
	return nil
}

func (client *Client) Call(ctx context.Context, method string, args ...interface{}) jsonrpc.Reply {

	var p interface{}
	if len(args) != 0 {
		p = args
	} else {
		empty := make([]interface{}, 0)

		p = empty
	}

	req := &jsonrpc.RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  p,
	}

	return &Result{
		client: client,
		req:    req,
		ctx:    ctx,
	}
}

// Send notifcation message
func (client *Client) Notification(ctx context.Context, method string, args ...interface{}) error {
	req := &jsonrpc.RPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  args,
	}

	buff, err := json.Marshal(req)

	if err != nil {
		return errors.Wrap(err, "marshal request error")
	}

	return client.Transport.Send(ctx, buff)
}

func (client *Client) send(ctx context.Context, req *jsonrpc.RPCRequest) (*jsonrpc.RPCResponse, error) {

	result := make(chan *jsonrpc.RPCResponse)
	client.Lock()
	seq := client.seq
	client.waitQ[client.seq] = result
	client.seq = seq + 1
	client.Unlock()

	req.ID = &seq

	client.D("jsonrpc call {@request}", req)

	buff, err := json.Marshal(req)

	if err != nil {
		return nil, errors.Wrap(err, "marshal request error")
	}

	if err := client.Transport.Send(ctx, buff); err != nil {
		client.tryGetWait(seq)
		close(result)
		return nil, err
	}

	timer := time.NewTimer(client.timeout)
	defer timer.Stop()
	defer close(result)

	select {
	case <-client.ctx.Done():
		client.tryGetWait(seq)
		return nil, errors.Wrap(jsonrpc.ErrClose, "cancel RPC %d by closing client", seq)
	case <-timer.C:
		client.tryGetWait(seq)
		return nil, errors.Wrap(jsonrpc.ErrTimeout, "RPC %d timeout", seq)
	case <-ctx.Done():
		client.tryGetWait(seq)
		return nil, errors.Wrap(ctx.Err(), "RPC %d canceled", seq)
	case resp := <-result:
		client.tryGetWait(seq)
		return resp, nil
	}
}

func (client *Client) tryGetWait(seq uint) (chan *jsonrpc.RPCResponse, bool) {
	client.Lock()
	defer client.Unlock()

	result, ok := client.waitQ[seq]

	if ok {
		delete(client.waitQ, seq)
	}

	return result, ok
}

// NewHTTPClient create jsonrpc client over http/https
func HTTPConnect(serviceURL string, opts ...ClientOpt) (jsonrpc.Client, error) {
	transport, err := transport.NewHTTPClientTransport(serviceURL)

	if err != nil {
		return nil, err
	}

	return New(append(opts, ClientTrans(transport))...)
}

// NewWebSocket create jsonrpc client over websocket
func WebSocketConnect(serviceURL string, opts ...ClientOpt) (jsonrpc.Client, error) {
	transport, err := transport.NewWebSocketClientTransport(serviceURL)

	if err != nil {
		return nil, err
	}

	return New(append(opts, ClientTrans(transport))...)
}
