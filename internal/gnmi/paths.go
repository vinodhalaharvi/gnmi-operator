package gnmi

import (
	gnmipb "github.com/openconfig/gnmi/proto/gnmi"
)

// InterfaceLeaf builds /interfaces/interface[name=iface]/{subtree}/{leaf}.
//
// The list key lives in PathElem.Key, not in the element name. The bracket
// syntax you see in documentation is a display convention; on the wire the
// key is a map. Getting this wrong is the single most common early mistake.
func InterfaceLeaf(iface, subtree, leaf string) *gnmipb.Path {
	return &gnmipb.Path{
		Elem: []*gnmipb.PathElem{
			{Name: "interfaces"},
			{Name: "interface", Key: map[string]string{"name": iface}},
			{Name: subtree},
			{Name: leaf},
		},
	}
}

// InterfaceMTU builds the mtu leaf under a chosen subtree (config or state).
func InterfaceMTU(iface, subtree string) *gnmipb.Path {
	return InterfaceLeaf(iface, subtree, "mtu")
}

// InterfaceEnabled builds the enabled leaf under a chosen subtree.
func InterfaceEnabled(iface, subtree string) *gnmipb.Path {
	return InterfaceLeaf(iface, subtree, "enabled")
}

// InterfaceDescription builds the description leaf under a chosen subtree.
func InterfaceDescription(iface, subtree string) *gnmipb.Path {
	return InterfaceLeaf(iface, subtree, "description")
}
