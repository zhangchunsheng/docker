package hooks

import (
	"testing"
)

func TestNewHook(t *testing.T) {
	if err := NewHook("/var/lib/docker/hooks", "/network/alloc/0_network.sh"); err != nil {
		t.Fatal(err)
	}

	hooks := registeredHooks["network"]
	if hooks == nil {
		for k := range registeredHooks {
			t.Log(k)
		}
		t.Log("hooks == nil")
		t.FailNow()
	}

	if len(hooks) != 1 {
		t.Logf("Expected number of hooks to be 1 got %d", len(hooks))
		t.FailNow()
	}

	h := hooks[0]

	if h.fileName != "0_network.sh" {
		t.Log(h.fileName)
		t.Fail()
	}

	if h.hookName != "network" {
		t.Log(h.hookName)
		t.Fail()
	}

	if h.action != "alloc" {
		t.Log(h.action)
		t.Fail()
	}

	if h.root != "/var/lib/docker/hooks" {
		t.Log(h.root)
		t.Fail()
	}
}
