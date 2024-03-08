package lib

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	resp "github.com/codecrafters-io/redis-starter-go/app/lib/encoding"
	"github.com/codecrafters-io/redis-starter-go/app/lib/repl"
	"io"
	"math/rand"
	"net"
	"time"
)

type ServerConfig struct {
	Host                   string
	Port                   int
	ConnectionReadTimeout  time.Duration
	ConnectionWriteTimeout time.Duration
	ReplicationConfig      *repl.ReplicationConfig
	ReplicaOf              *repl.ReplicaOf
}

func GetDefaultConfig() *ServerConfig {
	return DefaultConfig
}

var DefaultConfig = &ServerConfig{
	Host:                   "localhost",
	Port:                   6379,
	ConnectionReadTimeout:  time.Second * 2,
	ConnectionWriteTimeout: time.Second * 2,
	ReplicationConfig: &repl.ReplicationConfig{
		Role:               "master",
		ConnectedSlaves:    0,
		MasterReplOffset:   0,
		SecondReplOffset:   -1,
		ReplBacklogActive:  0,
		ReplBacklogSize:    1048576,
		ReplBacklogFirst:   0,
		ReplBacklogHistlen: 0,
	},
}

type Server struct {
	listener net.Listener
	close    chan struct{}
	handlers map[string]func(ctx context.Context, args *resp.Array) (interface{}, error)
	config   *ServerConfig
	replOf   *repl.ReplicaOf
}

func randomAlphanumericString(w io.Writer, len int) {
	source := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < len; i++ {
		chars := []byte{uint8(source.Intn(26) + 65), uint8(source.Intn(26) + 97), uint8(source.Intn(10) + 48)}
		w.Write([]byte{chars[source.Intn(3)]})
	}
}

func (s *Server) HandleInfo(ctx context.Context, args *resp.Array) (interface{}, error) {
	if len(args.A) < 1 {
		return nil, fmt.Errorf("ERR wrong number of arguments")
	}
	section, ok := args.A[0].(resp.BulkString)
	if !ok {
		return nil, fmt.Errorf("ERR invalid section type, expected string, got %T", (args.A)[0])
	}
	switch string(section.S) {
	case "replication":
		return s.config.ReplicationConfig, nil
	default:
		return nil, fmt.Errorf("ERR invalid section: %s", section.S)
	}
}

func getCommand(args *[]resp.Marshaller) (string, error) {
	if len(*args) == 0 {
		return "", fmt.Errorf("empty command")
	}

	switch command := (*args)[0].(type) {
	case resp.SimpleString:
		return command.S, nil
	case resp.BulkString:
		return string(command.S), nil
	}

	return "", fmt.Errorf("invalid command type: %T", (*args)[0])
}

func (s *Server) parser(con net.Conn) {
	defer con.Close()
	err := con.SetReadDeadline(time.Now().Add(s.config.ConnectionReadTimeout))
	if err != nil {
		resp.SimpleError{E: err.Error()}.MarshalRESP(con)
		fmt.Printf("error: %s", err.Error())
		return
	}
	err = con.SetWriteDeadline(time.Now().Add(s.config.ConnectionWriteTimeout))
	if err != nil {
		resp.SimpleError{E: err.Error()}.MarshalRESP(con)
		fmt.Printf("error: %s", err.Error())
		return
	}
	for {
		buff := make([]byte, 1024)
		_, err := con.Read(buff)
		if err != nil {
			resp.SimpleError{E: err.Error()}.MarshalRESP(con)
			return
		}
		reader := bufio.NewReader(bytes.NewReader(buff))
		var args resp.Array
		err = args.UnmarshalRESP(reader)
		if err != nil {
			resp.SimpleError{E: err.Error()}.MarshalRESP(con)
			return
		}
		command, err := getCommand(&args.A)
		if err != nil {
			resp.SimpleError{err.Error()}.MarshalRESP(con)
		}
		handler, ok := s.handlers[command]
		if !ok {
			resp.SimpleError{fmt.Sprintf("unknown command: %s", command)}.MarshalRESP(con)
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		args.A = args.A[1:]
		res, err := handler(ctx, &args)
		if err != nil {
			resp.SimpleError{err.Error()}.MarshalRESP(con)
		}
		resp.AnyResp{res, false}.MarshalRESP(con)
	}
}

func (s *Server) RegisterHandler(command string, handler func(context.Context, *resp.Array) (interface{}, error)) {
	s.handlers[command] = handler
}

func New(config *ServerConfig) (*Server, error) {
	if config == nil {
		config = DefaultConfig
	}

	if config.ReplicationConfig != nil {
		replID := bytes.NewBuffer(make([]byte, 40))
		randomAlphanumericString(replID, 40)
		config.ReplicationConfig.MasterReplid = string(replID.Bytes())
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", config.Host, config.Port))
	if err != nil {
		return nil, err
	}
	return &Server{
		listener: listener,
		close:    make(chan struct{}),
		handlers: make(map[string]func(ctx context.Context, args *resp.Array) (interface{}, error)),
		config:   config,
	}, err
}

func (s *Server) ListenAndServe() error {
	for {
		select {
		case <-s.close:
			return nil
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				return err
			}
			go s.parser(conn)
		}
	}

	panic("unreachable")
}

func (s *Server) Close() error {
	close(s.close)
	return s.listener.Close()
}
