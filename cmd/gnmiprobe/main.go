// Command gnmiprobe is the Phase 1 exercise: connect to a gNMI target, ask what
// it supports, read one value, change it, and read it back.
//
// The dial and TypedValue-decoding logic lives in internal/gnmi so the
// reconcilers use the same code as this diagnostic. What stays here is the
// flag surface, the human-readable prints, and the log.Fatalf failure mode
// that only belongs in a main.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/prototext"

	networkv1alpha1 "github.com/vinodhalaharvi/gnmi-operator/api/v1alpha1"
	"github.com/vinodhalaharvi/gnmi-operator/internal/gnmi"
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

	host, portStr, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatalf("parse target %q: %v", *addr, err)
	}
	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		log.Fatalf("parse port %q: %v", portStr, err)
	}

	spec := networkv1alpha1.DeviceSpec{
		Address:    host,
		Port:       int32(port),
		Insecure:   *skipTLS,
		ServerName: *hostname,
	}

	var caPEM []byte
	if !*skipTLS {
		caPEM, err = os.ReadFile(*caFile)
		if err != nil {
			log.Fatalf("read CA: %v", err)
		}
	}

	client, err := gnmi.Dial(ctx, spec, caPEM)
	if err != nil {
		log.Fatalf("dial %s: %v", *addr, err)
	}
	defer func() { _ = client.Close() }()

	if err := capabilities(ctx, client, *verbose); err != nil {
		log.Fatalf("Capabilities: %v", err)
	}

	// The two paths differ, and the difference matters. In OpenConfig the
	// config/ subtree is what you are allowed to write; state/ is what the
	// device reports. A well-behaved reconciler writes config and reads state,
	// which is also why desired and observed can legitimately disagree for a
	// while after a Set.
	configPath := gnmi.InterfaceMTU(*iface, "config")
	statePath := gnmi.InterfaceMTU(*iface, "state")

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

func capabilities(ctx context.Context, client *gnmi.Client, verbose bool) error {
	resp, err := client.Raw().Capabilities(ctx, &gnmipb.CapabilityRequest{})
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

func getMTU(ctx context.Context, client *gnmi.Client, p *gnmipb.Path, verbose bool) (uint64, error) {
	req := &gnmipb.GetRequest{
		Path:     []*gnmipb.Path{p},
		Type:     gnmipb.GetRequest_ALL,
		Encoding: gnmipb.Encoding_JSON_IETF,
	}
	resp, err := client.Raw().Get(ctx, req)
	if err != nil {
		return 0, err
	}
	if verbose {
		fmt.Println(prototext.Format(resp))
	}
	for _, n := range resp.GetNotification() {
		for _, u := range n.GetUpdate() {
			return gnmi.DecodeUint(u.GetVal())
		}
	}
	return 0, fmt.Errorf("no update in response")
}

func setMTU(ctx context.Context, client *gnmi.Client, p *gnmipb.Path, mtu uint64, verbose bool) error {
	req := &gnmipb.SetRequest{
		Update: []*gnmipb.Update{{
			Path: p,
			Val:  &gnmipb.TypedValue{Value: &gnmipb.TypedValue_UintVal{UintVal: mtu}},
		}},
	}
	resp, err := client.Raw().Set(ctx, req)
	if err != nil {
		return err
	}
	if verbose {
		fmt.Println(prototext.Format(resp))
	}
	return nil
}
