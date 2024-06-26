package replication

import (
	"bufio"
	"fmt"
	resp "github.com/codecrafters-io/redis-starter-go/app/lib/encoding"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type ReplicaOf struct {
	logger      *log.Logger
	r           *bufio.Reader
	conn        net.Conn
	propagation chan<- *REPLRequest
	db          *sync.Map
}

type REPLRequest struct {
	Logger *log.Logger
	Writer io.Writer
	Args   *resp.Array
	// Length in bytes of the Args array read from the reader
	N int
}

func (r *ReplicaOf) GetAddr() net.Addr {
	return r.conn.RemoteAddr()
}

// NewReplicaOf create replica of host by making a handshake sending listening port - port
func NewReplicaOf(db *sync.Map, host, port string, propagation chan<- *REPLRequest) (*ReplicaOf, error) {
	conn, err := net.DialTimeout("tcp", host, time.Second*10)
	if err != nil {
		return nil, err
	}
	logger := log.New(os.Stdout, fmt.Sprintf("replica %s of %s: ", port, host), log.LstdFlags|log.Lshortfile)
	logger.Printf("Connected to master %s", conn.RemoteAddr())
	repl := &ReplicaOf{
		logger:      logger,
		conn:        conn,
		r:           bufio.NewReader(conn),
		db:          db,
		propagation: propagation,
	}

	logger.Printf("Stage 1: PING")
	if err = repl.pingMaster(); err != nil {
		return nil, err
	}
	logger.Printf("Stage 2: PORT")
	if err = repl.replConfPort(port); err != nil {
		return nil, err
	}
	logger.Printf("Stage 3: CAPA")
	if err = repl.replConfCapa(); err != nil {
		return nil, err
	}
	logger.Printf("Stage 4: PSYNC")
	if err = repl.pSync(); err != nil {
		return nil, err
	}
	logger.Printf("Stage 5: Reading RDB")
	if _, err = repl.ReadRDB(); err != nil {
		logger.Printf("Error reading RDB: %s", err)
		return nil, err
	}

	logger.Printf("Stage 6: Listening for propagation")
	go repl.ListenAndAccept()
	return repl, nil
}

func (r *ReplicaOf) ReadRDB() (*resp.Rdb, error) {
	rdb := resp.NewRdb(r.db)
	if err := rdb.UnmarshalRESP(r.r); err != nil {
		return nil, err
	}

	return rdb, nil
}

func (r *ReplicaOf) pingMaster() error {
	var err error
	if _, err = (resp.Array{A: []resp.Marshaller{resp.BulkString{S: []byte("PING")}}}.MarshalRESP(r.conn)); err != nil {
		return err
	}
	res := resp.SimpleString{}
	if _, err := res.UnmarshalRESP(r.r); err != nil {
		return err
	}

	if res.S != "PONG" {
		return fmt.Errorf("expected PONG, got %q", res.S)
	}

	return nil
}

func (r *ReplicaOf) replConfPort(port string) error {
	var err error
	if _, err = (resp.Array{A: []resp.Marshaller{resp.BulkString{S: []byte("REPLCONF")}, resp.BulkString{S: []byte("listening-port")}, resp.BulkString{S: []byte(port)}}}.MarshalRESP(r.conn)); err != nil {
		return err
	}

	res := resp.SimpleString{}
	if _, err := res.UnmarshalRESP(r.r); err != nil {
		return err
	}

	if res.S != "OK" {
		return fmt.Errorf("expected OK, got %q", res.S)
	}

	return nil
}

func (r *ReplicaOf) replConfCapa() error {
	var err error
	if _, err = (resp.Array{A: []resp.Marshaller{resp.BulkString{S: []byte("REPLCONF")}, resp.BulkString{S: []byte("capa")}, resp.BulkString{S: []byte("psync2")}}}.MarshalRESP(r.conn)); err != nil {
		return err
	}
	res := resp.SimpleString{}
	if _, err := res.UnmarshalRESP(r.r); err != nil {
		return err
	}
	if res.S != "OK" {
		return fmt.Errorf("expected OK, got %q", res.S)
	}
	return nil
}

func (r *ReplicaOf) pSync() error {
	if _, err := (resp.Array{A: []resp.Marshaller{resp.BulkString{S: []byte("PSYNC")}, resp.BulkString{S: []byte("?")}, resp.BulkString{S: []byte("-1")}}}.MarshalRESP(r.conn)); err != nil {
		return err
	}
	res := resp.SimpleString{}
	if _, err := res.UnmarshalRESP(r.r); err != nil {
		return err
	}
	r.logger.Printf("Got PSYNC response: %s", res)
	return nil
}

func (r *ReplicaOf) ListenAndAccept() error {
	r.logger.Printf("Listening for propagation from %s", r.conn.RemoteAddr())
	for {
		var (
			args resp.Array
			err  error
			n    int
		)

		if n, err = args.UnmarshalRESP(r.r); err != nil && err != io.EOF {
			r.logger.Printf("Error reading request from master: %s", err)
			continue
		}

		r.logger.Printf("read %d, with %s bytes from %s", n, args, r.conn.RemoteAddr())
		if err == io.EOF {
			r.logger.Printf("Connection closed by %s", r.conn.RemoteAddr())
			return nil
		}

		r.propagation <- &REPLRequest{
			Writer: r.conn,
			Args:   &args,
			N:      n,
			Logger: r.logger,
		}

	}

}
