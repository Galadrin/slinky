package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/skip-mev/slinky/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var _ service.OracleService = (*GRPCClient)(nil)

// GRPCClient defines an implementation of a gRPC oracle client. This client can
// be used in ABCI++ calls where the application wants the oracle process to be
// run out-of-process. The client must be started upon app construction and
// stopped upon app shutdown/cleanup.
type GRPCClient struct {
	// address of remote oracle server
	addr string
	// underlying oracle client
	client service.OracleClient
	// underlying grpc connection
	conn *grpc.ClientConn
	// timeout for the client, Price requests will block for this duration.
	timeout time.Duration
	// mutex to protect the client
	mtx sync.Mutex
}

// NewGRPCClient creates a new grpc client of the oracle service, given the
// address of the oracle server and a timeout for the client.
func NewGRPCClient(addr string, t time.Duration) *GRPCClient {
	return &GRPCClient{
		addr:    addr,
		timeout: t,
		mtx:     sync.Mutex{},
	}
}

// Start starts the GRPC client. This method dials the remote oracle-service
// and errors if the connection fails.
func (c *GRPCClient) Start(ctx context.Context) error {
	conn, err := grpc.Dial(
		c.addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return fmt.Errorf("failed to dial oracle gRPC server: %w", err)
	}

	c.mtx.Lock()
	c.client = service.NewOracleClient(conn)
	c.conn = conn
	c.mtx.Unlock()

	return nil
}

// Stop stops the GRPC client. This method closes the connection to the remote.
func (c *GRPCClient) Stop(ctx context.Context) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	return c.conn.Close()
}

// Prices returns the prices from the remote oracle service. This method blocks for the timeout duration configured on the client,
// otherwise it returns the response from the remote oracle.
func (c *GRPCClient) Prices(ctx context.Context, req *service.QueryPricesRequest) (*service.QueryPricesResponse, error) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// set deadline on the context
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if c.client == nil {
		return nil, fmt.Errorf("oracle client not started")
	}

	return c.client.Prices(ctx, req, grpc.WaitForReady(true))
}
