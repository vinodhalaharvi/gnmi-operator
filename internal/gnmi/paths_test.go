package gnmi

import "testing"

func TestInterfaceMTUShape(t *testing.T) {
	elems := InterfaceMTU("eth1", "config").GetElem()
	if len(elems) != 4 {
		t.Fatalf("want 4 PathElem, got %d: %+v", len(elems), elems)
	}

	want := []struct {
		name string
		key  map[string]string
	}{
		{name: "interfaces"},
		{name: "interface", key: map[string]string{"name": "eth1"}},
		{name: "config"},
		{name: "mtu"},
	}
	for i, w := range want {
		if got := elems[i].GetName(); got != w.name {
			t.Errorf("elem[%d].name = %q, want %q", i, got, w.name)
		}
		gotKey := elems[i].GetKey()
		if w.key == nil {
			if len(gotKey) != 0 {
				t.Errorf("elem[%d].key = %v, want empty", i, gotKey)
			}
			continue
		}
		if len(gotKey) != len(w.key) {
			t.Errorf("elem[%d].key size = %d, want %d", i, len(gotKey), len(w.key))
		}
		for k, v := range w.key {
			if gotKey[k] != v {
				t.Errorf("elem[%d].key[%q] = %q, want %q", i, k, gotKey[k], v)
			}
		}
	}
}
