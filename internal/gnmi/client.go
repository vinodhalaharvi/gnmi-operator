// Package gnmi is a small, reconcile-safe client around openconfig gNMI.
//
// The controllers and the gnmiprobe command both dial the same targets and
// need the same TypedValue-decoding quirks. Keeping this in one place means we
// fix vendor variance once, and callers cannot accidentally log.Fatal from a
// reconcile loop.
package gnmi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	networkv1alpha1 "github.com/vinodhalaharvi/gnmi-operator/api/v1alpha1"
)

const defaultPort = 9339

// Client is a thin wrapper around a gNMI gRPC channel.
type Client struct {
	conn *grpc.ClientConn
	api  gnmipb.GNMIClient
}

// Dial builds a Client for the target described by spec. caPEM is the trust
// root used when TLS is on; it may be nil when spec.Insecure is true.
//
// The gRPC connection is created lazily (grpc.NewClient), so this call does
// not perform network I/O and ctx is not currently consumed. It is accepted
// for API stability and future use.
func Dial(ctx context.Context, spec networkv1alpha1.DeviceSpec, caPEM []byte) (*Client, error) {
	_ = ctx
	port := spec.Port
	if port == 0 {
		port = defaultPort
	}
	if spec.Address == "" {
		return nil, errors.New("spec.Address is empty")
	}
	addr := net.JoinHostPort(spec.Address, strconv.Itoa(int(port)))

	var (
		conn *grpc.ClientConn
		err  error
	)
	if spec.Insecure {
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		if len(caPEM) == 0 {
			return nil, errors.New("caPEM is required when spec.Insecure is false")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("no certificates found in caPEM")
		}
		serverName := spec.ServerName
		if serverName == "" {
			serverName = spec.Address
		}
		creds := credentials.NewTLS(&tls.Config{
			RootCAs:    pool,
			ServerName: serverName,
			MinVersion: tls.VersionTLS12,
		})
		conn, err = grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
	}
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return &Client{conn: conn, api: gnmipb.NewGNMIClient(conn)}, nil
}

// Close releases the underlying gRPC channel.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Raw exposes the underlying gNMI client. Prefer the typed helpers on Client;
// this exists for callers that need the raw request/response types, for
// example to dump a full proto message in a diagnostic tool.
func (c *Client) Raw() gnmipb.GNMIClient {
	return c.api
}

// Capabilities issues a CapabilityRequest and returns the version reported by
// the target and its supported encodings.
func (c *Client) Capabilities(ctx context.Context) (string, []gnmipb.Encoding, error) {
	resp, err := c.api.Capabilities(ctx, &gnmipb.CapabilityRequest{})
	if err != nil {
		return "", nil, err
	}
	return resp.GetGNMIVersion(), resp.GetSupportedEncodings(), nil
}

// GetUint reads a single integer leaf using JSON_IETF encoding and returns
// the decoded value. Handles UintVal, IntVal, JsonIetfVal and JsonVal because
// targets disagree about how to encode an integer leaf.
func (c *Client) GetUint(ctx context.Context, path *gnmipb.Path) (uint64, error) {
	v, err := c.getLeaf(ctx, path)
	if err != nil {
		return 0, err
	}
	return DecodeUint(v)
}

// SetUint writes an integer leaf using TypedValue_UintVal (Update semantics).
func (c *Client) SetUint(ctx context.Context, path *gnmipb.Path, v uint64) error {
	req := &gnmipb.SetRequest{
		Update: []*gnmipb.Update{{
			Path: path,
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_UintVal{UintVal: v}},
		}},
	}
	_, err := c.api.Set(ctx, req)
	return err
}

// GetBool reads a boolean leaf using JSON_IETF encoding.
func (c *Client) GetBool(ctx context.Context, path *gnmipb.Path) (bool, error) {
	v, err := c.getLeaf(ctx, path)
	if err != nil {
		return false, err
	}
	return DecodeBool(v)
}

// SetBool writes a boolean leaf using TypedValue_BoolVal (Update semantics).
func (c *Client) SetBool(ctx context.Context, path *gnmipb.Path, v bool) error {
	req := &gnmipb.SetRequest{
		Update: []*gnmipb.Update{{
			Path: path,
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_BoolVal{BoolVal: v}},
		}},
	}
	_, err := c.api.Set(ctx, req)
	return err
}

// IsNotFound reports whether err is a gRPC NotFound. Our gnxi target returns
// NotFound for a whole absent subtree, and callers need to distinguish
// "leaf absent" (a Ready=False condition with a specific reason) from
// "device unreachable" (a Ready=Unknown condition).
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.NotFound
}

func (c *Client) getLeaf(ctx context.Context, path *gnmipb.Path) (*gnmipb.TypedValue, error) {
	req := &gnmipb.GetRequest{
		Path:     []*gnmipb.Path{path},
		Type:     gnmipb.GetRequest_ALL,
		Encoding: gnmipb.Encoding_JSON_IETF,
	}
	resp, err := c.api.Get(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, n := range resp.GetNotification() {
		for _, u := range n.GetUpdate() {
			return u.GetVal(), nil
		}
	}
	return nil, errors.New("no update in response")
}

// DecodeUint returns the uint64 value of a gNMI TypedValue. It tolerates the
// four encodings we've seen in the wild — this is the vendor-variance problem
// showing up on day one.
func DecodeUint(v *gnmipb.TypedValue) (uint64, error) {
	switch t := v.GetValue().(type) {
	case *gnmipb.TypedValue_UintVal:
		return t.UintVal, nil
	case *gnmipb.TypedValue_IntVal:
		return uint64(t.IntVal), nil
	case *gnmipb.TypedValue_JsonIetfVal:
		return parseUint(string(t.JsonIetfVal))
	case *gnmipb.TypedValue_JsonVal:
		return parseUint(string(t.JsonVal))
	default:
		return 0, fmt.Errorf("unexpected TypedValue type %T", t)
	}
}

// DecodeBool returns the bool value of a gNMI TypedValue, tolerating JSON
// encodings that some targets use.
func DecodeBool(v *gnmipb.TypedValue) (bool, error) {
	switch t := v.GetValue().(type) {
	case *gnmipb.TypedValue_BoolVal:
		return t.BoolVal, nil
	case *gnmipb.TypedValue_JsonIetfVal:
		return parseBool(string(t.JsonIetfVal))
	case *gnmipb.TypedValue_JsonVal:
		return parseBool(string(t.JsonVal))
	default:
		return false, fmt.Errorf("unexpected TypedValue type %T", t)
	}
}

func parseUint(raw string) (uint64, error) {
	s := strings.Trim(strings.TrimSpace(raw), `"`)
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("value %q is not an unsigned integer: %w", raw, err)
	}
	return n, nil
}

func parseBool(raw string) (bool, error) {
	s := strings.Trim(strings.TrimSpace(raw), `"`)
	switch s {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("value %q is not a boolean", raw)
}
