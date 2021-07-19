package jsonrpc

import (
	"context"
	"net/http"
	"testing"

	"github.com/libs4go/scf4go"
	_ "github.com/libs4go/scf4go/codec/json" //
	"github.com/libs4go/scf4go/reader/memory"
	"github.com/libs4go/slf4go"

	_ "github.com/libs4go/slf4go/backend/console" //
	"github.com/stretchr/testify/require"
)

type rpcServer struct {
}

func (s *rpcServer) SayHello(msg string, code int) (string, error) {
	return msg, nil
}

var configFile = `
{
    "default": {
		"backend": "console"
	}
}
`

func init() {
	config := scf4go.New()

	err := config.Load(memory.New(memory.Data(configFile, "json")))

	if err != nil {
		panic(err)
	}

	err = slf4go.Config(config)

	if err != nil {
		panic(err)
	}
}

func TestHttp(t *testing.T) {

	defer slf4go.Sync()

	server, err := NewHTPPServer(Dispatch(&rpcServer{}))

	require.NoError(t, err)

	defer server.Close()

	go func() {
		err := http.ListenAndServe(":8080", server)

		require.NoError(t, err)
	}()

	client, err := NewHTTPClient("http://localhost:8080")

	require.NoError(t, err)

	var echo string

	err = client.Call(context.Background(), "SayHello", "Hello", 1).Unmarshal(&echo)

	require.NoError(t, err)

	require.Equal(t, echo, "Hello")
}
