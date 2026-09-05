// Command gnmiprobe is the Phase 1 exercise: connect to a gNMI target, ask what
// it supports, read one value, change it, and read it back.
//
// Everything here is built from raw protobuf structures on purpose. There are
// convenience libraries that would collapse this file to about twenty lines,
// and using one now would mean never learning what a SetRequest looks like.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/prototext"
)

func main() {
	var (
		addr     = flag.String("target", "127.0.0.1:9339", "target address host:port")
		caFile   = flag.String("ca", "certs/ca.crt", "CA certificate for the target")
		hostname = flag.String("target-name", "target.lab", "name to verify in the target certificate")
		skipTLS  = flag.Bool("insecure", false, "skip TLS entirely (plaintext gRPC)")
		user     = flag.String("username", "", "username, sent as gRPC metadata")
		pass     = flag.String("password", "", "password, sent as gRPC metadata")
		iface    = flag.String("interface", "eth1", "interface name to probe")
		newMTU   = flag.Uint("mtu", 9000, "MTU to write during the Set step")
		verbose  = flag.Bool("v", false, "dump full protobuf messages")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if *user != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "username", *user, "password", *pass)
	}

	conn, err := dial(*addr, *caFile, *hostname, *skipTLS)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer func() { _ = conn.Close() }()
	client := gnmipb.NewGNMIClient(conn)

	if err := capabilities(ctx, client, *verbose); err != nil {
		log.Fatalf("Capabilities: %v", err)
	}

	// The two paths differ, and the difference matters. In OpenConfig the
	// config/ subtree is what you are allowed to write; state/ is what the
	// device reports. A well-behaved reconciler writes config and reads state,
	// which is also why desired and observed can legitimately disagree for a
	// while after a Set.
	configPath := mtuPath(*iface, "config")
	statePath := mtuPath(*iface, "state")

	before, err := getMTU(ctx, client, statePath, *verbose)
	if err != nil {
		log.Printf("Get (state) failed: %v — falling back to the config path", err)
		if before, err = getMTU(ctx, client, configPath, *verbose); err != nil {
			log.Fatalf("Get: %v", err)
		}
	}
	fmt.Printf("observed mtu = %d\n", before)

	if uint64(*newMTU) == before {
		fmt.Println("desired == observed; a reconciler would stop here (this is idempotency)")
		return
	}

	if err := setMTU(ctx, client, configPath, uint64(*newMTU), *verbose); err != nil {
		log.Fatalf("Set: %v", err)
	}
	fmt.Printf("wrote mtu = %d\n", *newMTU)

	after, err := getMTU(ctx, client, statePath, *verbose)
	if err != nil {
		if after, err = getMTU(ctx, client, configPath, *verbose); err != nil {
			log.Fatalf("verify Get: %v", err)
		}
	}
	fmt.Printf("verified mtu = %d\n", after)

	if after != uint64(*newMTU) {
		fmt.Println("target accepted the Set but reports something else — " +
			"this is exactly the case status conditions exist for")
		os.Exit(1)
	}
}

// dial builds the gRPC channel. gNMI is TLS by default; plaintext is a lab-only
// convenience and most real targets will refuse it.
func dial(addr, caFile, hostname string, skip bool) (*grpc.ClientConn, error) {
	if skip {
		return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no certificates found in %s", caFile)
	}
	creds := credentials.NewTLS(&tls.Config{
		RootCAs:    pool,
		ServerName: hostname,
		MinVersion: tls.VersionTLS12,
	})
	return grpc.NewClient(addr, grpc.WithTransportCredentials(creds))
}

// mtuPath builds /interfaces/interface[name=X]/{config,state}/mtu.
//
// Note that the list key lives in PathElem.Key, not in the element name. The
// bracket syntax you see in documentation is a display convention; on the wire
// it is a map. Getting this wrong is the single most common early mistake.
func mtuPath(iface, subtree string) *gnmipb.Path {
	return &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": iface}},
			{Name: subtree},
			{Name: "mtu"},
		},
	}
}

func capabilities(ctx context.Context, c gnmipb.GNMIClient, verbose bool) error {
	resp, err := c.Capabilities(ctx, &gnmipb.CapabilityRequest{})
	if err != nil {
		return err
	}
	fmt.Printf("gNMI version %s, %d models, encodings %v\n",
		resp.GetGNMIVersion(), len(resp.GetSupportedModels()), resp.GetSupportedEncodings())
	if verbose {
		fmt.Println(prototext.Format(resp))
	}
	// Worth checking rather than assuming: if JSON_IETF is absent, the encoding
	// you pick for Set has to change.
	for _, e := range resp.GetSupportedEncodings() {
		if e == gnmipb.Encoding_JSON_IETF {
			return nil
		}
	}
	fmt.Println("note: target does not advertise JSON_IETF")
	return nil
}

func getMTU(ctx context.Context, c gnmipb.GNMIClient, p *gnmipb.Path, verbose bool) (uint64, error) {
	req := &gnmipb.GetRequest{
		Path:     []*gnmipb.Path{p},
		Type:     gnmipb.GetRequest_ALL,
		Encoding: gnmipb.Encoding_JSON_IETF,
	}
	resp, err := c.Get(ctx, req)
	if err != nil {
		return 0, err
	}
	if verbose {
		fmt.Println(prototext.Format(resp))
	}
	// A GetResponse is a list of Notifications, each holding a list of Updates.
	// Even a single-leaf read has this shape, which is a hint about how much
	// normalisation a reconciler will eventually need.
	for _, n := range resp.GetNotification() {
		for _, u := range n.GetUpdate() {
			return typedValueToUint(u.GetVal())
		}
	}
	return 0, fmt.Errorf("no update in response")
}

func setMTU(ctx context.Context, c gnmipb.GNMIClient, p *gnmipb.Path, mtu uint64, verbose bool) error {
	// Update merges, Replace overwrites the subtree, Delete removes it. Update
	// is the safe choice for a single leaf; Replace is what you reach for when
	// you want the device to look exactly like your intent and nothing else.
	req := &gnmipb.SetRequest{
		Update: []*gnmipb.Update{{
			Path: p,
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_UintVal{UintVal: mtu}},
		}},
	}
	resp, err := c.Set(ctx, req)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Println(prototext.Format(resp))
	}
	return nil
}

// typedValueToUint exists because targets disagree about how to encode an
// integer leaf. Handling three cases here is not defensive programming; it is
// the vendor-variance problem showing up on day one.
func typedValueToUint(v *gnmipb.TypedValue) (uint64, error) {
	switch t := v.GetValue().(type) {
	case *gnmipb.TypedValue_UintVal:
		return t.UintVal, nil
	case *gnmipb.TypedValue_IntVal:
		return uint64(t.IntVal), nil
	case *gnmipb.TypedValue_JsonIetfVal:
		var n uint64
		if _, err := fmt.Sscanf(string(t.JsonIetfVal), "%d", &n); err != nil {
			return 0, fmt.Errorf("json_ietf value %q is not an integer", t.JsonIetfVal)
		}
		return n, nil
	case *gnmipb.TypedValue_JsonVal:
		var n uint64
		if _, err := fmt.Sscanf(string(t.JsonVal), "%d", &n); err != nil {
			return 0, fmt.Errorf("json value %q is not an integer", t.JsonVal)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unexpected TypedValue type %T", t)
	}
}
