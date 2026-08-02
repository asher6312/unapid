package dockerctl

import (
	"context"
	"strings"
	"testing"

	"github.com/asher6312/unapid/internal/process"
)

type fakeRunner struct {
	results  map[string]process.Result
	commands []string
}

func commandKey(name string, args []string) string {
	return name + "\x00" + strings.Join(args, "\x00")
}

func (f *fakeRunner) Capture(_ context.Context, name string, args ...string) (process.Result, error) {
	key := commandKey(name, args)
	f.commands = append(f.commands, key)
	if result, exists := f.results[key]; exists {
		return result, nil
	}
	return process.Result{}, nil
}

func (*fakeRunner) Interactive(context.Context, []string, string, ...string) error {
	return nil
}

func TestPickNetworkUsesStableSafetyPriority(t *testing.T) {
	tests := []struct {
		networks []string
		want     string
	}{
		{[]string{"bridge", "proxy", "unapid_private"}, "unapid_private"},
		{[]string{"bridge", "n8n_default", "proxy"}, "proxy"},
		{[]string{"bridge", "zeta", "alpha"}, "alpha"},
		{[]string{"bridge"}, "bridge"},
	}
	for _, test := range tests {
		got, err := PickNetwork(test.networks)
		if err != nil || got != test.want {
			t.Fatalf("PickNetwork(%v) = %q, %v; want %q", test.networks, got, err, test.want)
		}
	}
}

func TestAcquireCreatesAndConnectsOnlyManagedNetwork(t *testing.T) {
	runner := &fakeRunner{results: map[string]process.Result{
		commandKey("docker", []string{"network", "inspect", "unapid_private", "--format", "{{json .Labels}}"}): {Code: 1, Stderr: "Error: No such network"},
		commandKey("docker", []string{"inspect", "n8n", "--format", "{{json .NetworkSettings.Networks}}"}):     {Stdout: `{"bridge":{}}`, Code: 0},
	}}
	client := New(runner)
	lease, err := client.Acquire(context.Background(), "bridge", "n8n")
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Created || !lease.Connected || lease.Name != "unapid_private" {
		t.Fatalf("unexpected lease: %#v", lease)
	}
	wantCreate := commandKey("docker", []string{"network", "create", "--driver", "bridge", "--label", "io.unapid.network.managed=true", "unapid_private"})
	wantConnect := commandKey("docker", []string{"network", "connect", "unapid_private", "n8n"})
	joined := strings.Join(runner.commands, "\n")
	if !strings.Contains(joined, wantCreate) || !strings.Contains(joined, wantConnect) {
		t.Fatalf("managed network commands missing:\n%s", joined)
	}
	if strings.Contains(joined, "restart") || strings.Contains(joined, "stop") {
		t.Fatalf("n8n mutation command found:\n%s", joined)
	}
}

func TestAcquireRejectsReservedUnmanagedNetwork(t *testing.T) {
	runner := &fakeRunner{results: map[string]process.Result{
		commandKey("docker", []string{"network", "inspect", "unapid_private", "--format", "{{json .Labels}}"}): {Stdout: `{}`, Code: 0},
	}}
	_, err := New(runner).Acquire(context.Background(), "unapid_private", "n8n")
	if err == nil || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("expected unmanaged-network refusal, got %v", err)
	}
}

func TestParseServiceStatesAndPublicationSafety(t *testing.T) {
	states, err := ParseServiceStates("{\"Service\":\"api\",\"State\":\"running\",\"Health\":\"healthy\",\"Publishers\":[{\"URL\":\"\",\"TargetPort\":8317,\"PublishedPort\":0}]}\n")
	if err != nil || len(states) != 1 || !InternalOnly(states) {
		t.Fatalf("unexpected state parse: %#v, %v", states, err)
	}
	states[0].Publishers[0].PublishedPort = 8317
	if InternalOnly(states) {
		t.Fatal("published host port was accepted")
	}
}
