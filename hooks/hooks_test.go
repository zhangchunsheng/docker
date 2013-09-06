package hooks

import (
	"testing"
)

func TestHookName(t *testing.T) {
	h := &Hook{"network/alloc/0_network.sh", "/var/lib/docker/hooks", "alloc"}

	if h.Hook() != "network" {
		t.Fail()
	}

	if h.action != "alloc" {
		t.Fail()
	}
}
