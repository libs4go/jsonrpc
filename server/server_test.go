package server

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/libs4go/jsonrpc/client"
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

func (s *rpcServer) OptionCall(msg string, code *int) (string, error) {
	return msg, nil
}

func (s *rpcServer) ErrorCall() (string, error) {
	return "", fmt.Errorf("ErrorCall")
}

var configFile = `
{
    "default": {
		"backend": "console"
	},
	"backend": {
		"console": {
			"formatter": {
				"timestamp":"Mon, 02 Jan 2006 15:04:05 -0700",
				"output":"@t @l @s @m"
			}
		}
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

	server, err := NewHTTPServer(&rpcServer{})

	require.NoError(t, err)

	go func() {
		err := http.ListenAndServe(":8080", server)

		require.NoError(t, err)
	}()

	client, err := client.NewHTTPClient("http://localhost:8080")

	require.NoError(t, err)

	var echo string

	err = client.Call(context.Background(), "SayHello", "Hello", 1).Join(&echo)

	require.NoError(t, err)

	require.Equal(t, echo, "Hello")

	err = client.Call(context.Background(), "OptionCall", "Hello").Join(&echo)

	require.NoError(t, err)

	require.Equal(t, echo, "Hello")

	err = client.Call(context.Background(), "ErrorCall").Join(&echo)

	require.Error(t, err)
}
