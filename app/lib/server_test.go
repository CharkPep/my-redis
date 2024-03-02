package lib

import (
	"bytes"
	"fmt"
	resp "github.com/codecrafters-io/redis-starter-go/app/lib/encoding"
	"github.com/codecrafters-io/redis-starter-go/app/lib/handlers"
	"net"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	server, err := New(nil)
	if err != nil {
		panic(err)
	}
	server.RegisterHandler("ping", handlers.Ping)
	server.RegisterHandler("echo", handlers.Echo)
	go server.ListenAndServe()
	code := m.Run()
	err = server.Close()
	if err != nil {
		fmt.Println("Failed to terminate")
		os.Exit(1)
	}
	fmt.Println("Terminated successfully")
	os.Exit(code)
}

func TestServer_getCommand(t *testing.T) {
	type testCases struct {
		input    []resp.RespMarshaler
		expected string
	}
	tests := []testCases{
		{
			input:    []resp.RespMarshaler{resp.SimpleString{"hello"}},
			expected: "hello",
		},
		{
			input:    []resp.RespMarshaler{resp.BulkString{S: []byte("hello")}},
			expected: "hello",
		},
	}
	for _, test := range tests {
		result, _ := getCommand(&test.input)
		if result != test.expected {
			t.Fatalf("expected %v, got %v", test.expected, result)
		}
	}
}

func TestServerShouldAcceptConnection_ListenAndServe(t *testing.T) {
	_, err := net.Dial("tcp", "localhost:6379")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
}

func TestServerShouldReturnPong_ListenAndServer(t *testing.T) {
	conn, err := net.Dial("tcp", "localhost:6379")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	conn.Write([]byte("*1\r\n$4\r\nping\r\n"))
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if string(buf[:n]) != "+PONG\r\n" {
		t.Errorf("expected +PONG\r\n, got %s", string(buf[:n]))
	}
}

func TestServerShouldReturnEcho_ListenAndServer(t *testing.T) {
	type testCase struct {
		args   resp.RespMarshaler
		output string
	}
	tests := []testCase{
		{
			args:   resp.RespArray{A: []resp.RespMarshaler{resp.BulkString{S: []byte("echo")}, resp.BulkString{S: []byte("foo")}}},
			output: "$3\r\nfoo\r\n",
		},
		{
			args: resp.RespArray{A: []resp.RespMarshaler{resp.BulkString{S: []byte("echo")},
				resp.BulkString{S: []byte("foo")}, resp.BulkString{S: []byte("bar")}}},
			output: "$3\r\nfoo\r\n",
		},
		{
			args:   resp.RespArray{A: []resp.RespMarshaler{resp.BulkString{S: []byte("echo")}, resp.BulkString{S: []byte("apples")}}},
			output: "$6\r\napples\r\n",
		},
		{
			args:   resp.RespArray{A: []resp.RespMarshaler{resp.BulkString{S: []byte("echo")}}},
			output: "-ERR wrong number of arguments for command\r\n",
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("echo:%d", i), func(ts *testing.T) {
			test := test
			ts.Parallel()
			conn, err := net.Dial("tcp", "localhost:6379")
			if err != nil {
				ts.Errorf("unexpected error: %s", err)
			}
			buff := bytes.NewBuffer([]byte{})
			err = test.args.MarshalRESP(buff)
			t.Logf("%q", buff.String())
			if err != nil {
				ts.Errorf("unexpected error: %s", err)
			}
			if _, err = conn.Write(buff.Bytes()); err != nil {
				ts.Errorf("unexpected error: %s", err)
			}
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				ts.Errorf("unexpected error: %s", err)
			}
			if string(buf[:n]) != test.output {
				ts.Errorf("expected %q, got %q, length %d", test.output, string(buf[:n]), n)
			}
		})
	}
}
