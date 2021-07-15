package jsonrpc

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/libs4go/errors"
	"github.com/libs4go/slf4go"
)

type Result struct {
	client *Client
	req    *RPCRequest
	ctx    context.Context
}

func (result *Result) Unmarshal(resultObject interface{}) error {
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
	Transport Transport                  // Client transport
	seq       uint                       // request seq
	waitQ     map[uint]chan *RPCResponse // waitQ
	timeout   time.Duration              // rpc global timeout
	ctx       context.Context
	cancelF   context.CancelFunc
}

// ClientOpt .
type ClientOpt func(client *Client)

// ClientTransport set client transport
func ClientTransport(transport Transport) ClientOpt {
	return func(client *Client) {
		client.Transport = transport
	}
}

// ClientTimeout set client timeout
func ClientTimeout(duration time.Duration) ClientOpt {
	return func(client *Client) {
		client.timeout = duration
	}
}

func newClient(ctx context.Context, options ...ClientOpt) *Client {

	newCtx, cancelF := context.WithCancel(ctx)

	client := &Client{
		Logger:  slf4go.Get("jsonrpc"),
		waitQ:   make(map[uint]chan *RPCResponse),
		timeout: time.Second * 60,
		seq:     1,
		cancelF: cancelF,
		ctx:     newCtx,
	}

	for _, opt := range options {
		opt(client)
	}

	go client.runLoop()

	return client
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

			var resp *RPCResponse

			err := json.Unmarshal(buff, &resp)

			if err != nil {
				client.E("unmarshal resp {@buff} err {@err}", buff, err)
				continue
			}

			result, ok := client.tryGetWait(resp.ID)

			if !ok {
				client.E("match request not found for {@resp}", resp)
				continue
			}

			client.sendResult(result, resp)
		}
	}

}

func (client *Client) sendResult(result chan *RPCResponse, resp *RPCResponse) {
	defer func() {
		if err := recover(); err != nil {
			client.E("send resp {@resp} error {@err}", resp, err)
		}
	}()

	result <- resp
}

func (client *Client) clear() {
	transportCloser, ok := client.Transport.(TransportCloser)

	if ok {
		transportCloser.Close()
	}
}

func (client *Client) Close() error {
	client.cancelF()
	return nil
}

func (client *Client) Call(ctx context.Context, method string, args ...interface{}) *Result {

	var p interface{}
	if len(args) != 0 {
		p = args
	} else {
		empty := make([]interface{}, 0)

		p = empty
	}

	req := &RPCRequest{
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
func (client *Client) Notification(method string, args ...interface{}) error {
	req := &RPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  args,
	}

	buff, err := json.Marshal(req)

	if err != nil {
		return errors.Wrap(err, "marshal request error")
	}

	return client.Transport.Send(buff)
}

func (client *Client) send(ctx context.Context, req *RPCRequest) (*RPCResponse, error) {

	result := make(chan *RPCResponse)
	client.Lock()
	seq := client.seq
	client.waitQ[client.seq] = result
	client.seq = seq + 1
	client.Unlock()

	req.ID = uint(seq)

	client.D("jsonrpc call {@request}", req)

	buff, err := json.Marshal(req)

	if err != nil {
		return nil, errors.Wrap(err, "marshal request error")
	}

	if err := client.Transport.Send(buff); err != nil {
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
		return nil, errors.Wrap(ErrClose, "cancel RPC %d by closing client", seq)
	case <-timer.C:
		client.tryGetWait(seq)
		return nil, errors.Wrap(ErrTimeout, "RPC %d timeout", seq)
	case <-ctx.Done():
		client.tryGetWait(seq)
		return nil, errors.Wrap(ctx.Err(), "RPC %d canceled", seq)
	case resp := <-result:
		client.tryGetWait(seq)
		return resp, nil
	}
}

func (client *Client) tryGetWait(seq uint) (chan *RPCResponse, bool) {
	client.Lock()
	defer client.Unlock()

	result, ok := client.waitQ[seq]

	if ok {
		delete(client.waitQ, seq)
	}

	return result, ok
}

// HTTPClient create jsonrpc client over http/https
func HTTPClient(ctx context.Context, serviceURL string, opts ...ClientOpt) (*Client, error) {
	transport, err := NewHTTPClient(serviceURL)

	if err != nil {
		return nil, err
	}

	return newClient(ctx, append(opts, ClientTransport(transport))...), nil
}