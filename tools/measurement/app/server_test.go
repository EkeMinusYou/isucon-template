package app

import (
	"net"
	"testing"
	"time"
)

func TestStartReportsConfigurationAndBindErrors(t *testing.T) {
	for _, options := range []Options{
		{MaxProfileSeconds: -1}, {MaxProfileSeconds: 3601},
		{Pools: []Pool{{Name: "missing"}}},
	} {
		if _, err := Start(options); err == nil {
			t.Fatalf("accepted invalid options: %+v", options)
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if _, err := Start(Options{Address: listener.Addr().String()}); err == nil {
		t.Fatal("occupied address did not fail synchronously")
	}
	server, err := Start(Options{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if server.WriteTimeout != 195*time.Second {
		t.Fatalf("profile response timeout=%s", server.WriteTimeout)
	}
}
