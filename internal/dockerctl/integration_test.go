package dockerctl

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/asher6312/unapid/internal/buildinfo"
	"github.com/asher6312/unapid/internal/process"
)

func TestLiveBridgeLifecycle(t *testing.T) {
	if os.Getenv("UNAPID_DOCKER_INTEGRATION") != "1" {
		t.Skip("set UNAPID_DOCKER_INTEGRATION=1 for the isolated Docker test")
	}
	container := os.Getenv("UNAPID_TEST_CONTAINER")
	if !validName(container) {
		t.Fatal("UNAPID_TEST_CONTAINER is invalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := New(process.Local{})
	lease, err := client.Acquire(ctx, "bridge", container)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Name != buildinfo.ManagedNetwork || !lease.Created || !lease.Connected {
		t.Fatalf("unexpected managed-network lease: %#v", lease)
	}
	networks, err := client.Networks(ctx, container)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, network := range networks {
		found = found || network == buildinfo.ManagedNetwork
	}
	if !found {
		t.Fatal("managed network was not attached")
	}
	if err := client.Rollback(ctx, lease); err != nil {
		t.Fatal(err)
	}
	networks, err = client.Networks(ctx, container)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range networks {
		if network == buildinfo.ManagedNetwork {
			t.Fatal("managed network remained after rollback")
		}
	}
}
