package jsonrpc

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type rpcServer struct {
}

func (s *rpcServer) SayHello(msg string, code int) (string, error) {
	println("=============")
	return msg, nil
}

func init() {

}

func TestHttp(t *testing.T) {
	server, httpServer, err := NewHTPPServer(Dispatch(&rpcServer{}))

	require.NoError(t, err)

	defer server.Close()

	go func() {
		err := http.ListenAndServe(":8080", httpServer)

		require.NoError(t, err)
	}()

	client, err := NewHTTPClient("http://localhost:8080")

	require.NoError(t, err)

	var echo string

	err = client.Call(context.Background(), "SayHello", "Hello", 1).Unmarshal(&echo)

	require.NoError(t, err)

	require.Equal(t, echo, "Hello")
}
