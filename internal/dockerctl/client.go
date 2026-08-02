package dockerctl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/asher6312/unapid/internal/buildinfo"
	"github.com/asher6312/unapid/internal/process"
)

var officialN8N = regexp.MustCompile(`(?:^|/)n8nio/n8n(?:[:@]|$)`)

type Container struct {
	ID    string
	Image string
	Name  string
}

type Publisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
}

type ServiceState struct {
	Service    string      `json:"Service"`
	State      string      `json:"State"`
	Health     string      `json:"Health"`
	Publishers []Publisher `json:"Publishers"`
}

type Lease struct {
	Name      string
	Container string
	Created   bool
	Connected bool
}

type Client struct {
	runner process.Runner
}

func New(runner process.Runner) *Client {
	return &Client{runner: runner}
}

func failed(label string, result process.Result) error {
	detail := process.Detail(result)
	if detail == "" {
		return fmt.Errorf("%s failed", label)
	}
	return fmt.Errorf("%s failed: %s", label, detail)
}

func (c *Client) capture(ctx context.Context, args ...string) (process.Result, error) {
	return c.runner.Capture(ctx, "docker", args...)
}

func (c *Client) Require(ctx context.Context) error {
	cli, err := c.capture(ctx, "--version")
	if err != nil {
		return errors.New("Docker Engine is required")
	}
	if cli.Code != 0 {
		return failed("Docker CLI check", cli)
	}
	if strings.Contains(strings.ToLower(cli.Stdout+cli.Stderr), "podman") {
		return errors.New("Docker Engine is required; Podman Docker emulation is not supported")
	}
	for _, check := range []struct {
		label string
		args  []string
	}{
		{"Docker server check", []string{"version", "--format", "{{.Server.Version}}"}},
		{"Docker Compose check", []string{"compose", "version", "--short"}},
	} {
		result, runErr := c.capture(ctx, check.args...)
		if runErr != nil {
			return fmt.Errorf("%s failed: %w", check.label, runErr)
		}
		if result.Code != 0 || strings.TrimSpace(result.Stdout) == "" {
			return failed(check.label, result)
		}
	}
	return nil
}

func (c *Client) RunningN8N(ctx context.Context) ([]Container, error) {
	result, err := c.capture(ctx, "ps", "--filter", "status=running", "--format", "{{json .}}")
	if err != nil {
		return nil, fmt.Errorf("discover n8n: %w", err)
	}
	if result.Code != 0 {
		return nil, failed("n8n discovery", result)
	}
	var containers []Container
	scanner := bufio.NewScanner(strings.NewReader(result.Stdout))
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var row struct {
			ID    string `json:"ID"`
			Image string `json:"Image"`
			Names string `json:"Names"`
			State string `json:"State"`
		}
		if err := json.Unmarshal([]byte(scanner.Text()), &row); err != nil {
			return nil, errors.New("Docker returned an unreadable container list")
		}
		if row.State == "running" && officialN8N.MatchString(row.Image) && validName(row.Names) {
			containers = append(containers, Container{ID: row.ID, Image: row.Image, Name: row.Names})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("Docker returned an unreadable container list")
	}
	sort.Slice(containers, func(i, j int) bool { return containers[i].Name < containers[j].Name })
	return containers, nil
}

func validName(value string) bool {
	if value == "" || value[0] == '-' {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (c *Client) Networks(ctx context.Context, container string) ([]string, error) {
	if !validName(container) {
		return nil, errors.New("the n8n container name is invalid")
	}
	result, err := c.capture(ctx, "inspect", container, "--format", "{{json .NetworkSettings.Networks}}")
	if err != nil {
		return nil, fmt.Errorf("inspect n8n networks: %w", err)
	}
	if result.Code != 0 {
		return nil, failed("Docker network discovery", result)
	}
	var attached map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &attached); err != nil {
		return nil, errors.New("Docker returned unreadable network information")
	}
	networks := make([]string, 0, len(attached))
	for name := range attached {
		if validName(name) && name != "host" && name != "none" {
			networks = append(networks, name)
		}
	}
	sort.Strings(networks)
	return networks, nil
}

func PickNetwork(networks []string) (string, error) {
	set := make(map[string]bool, len(networks))
	for _, name := range networks {
		if validName(name) && name != "host" && name != "none" {
			set[name] = true
		}
	}
	if set[buildinfo.ManagedNetwork] {
		return buildinfo.ManagedNetwork, nil
	}
	if set["proxy"] {
		return "proxy", nil
	}
	var userDefined []string
	for name := range set {
		if name != "bridge" {
			userDefined = append(userDefined, name)
		}
	}
	sort.Strings(userDefined)
	if len(userDefined) > 0 {
		return userDefined[0], nil
	}
	if set["bridge"] {
		return "bridge", nil
	}
	return "", errors.New("the n8n container has no usable Docker network")
}

func (c *Client) networkLabels(ctx context.Context, network string) (map[string]string, bool, error) {
	result, err := c.capture(ctx, "network", "inspect", network, "--format", "{{json .Labels}}")
	if err != nil {
		return nil, false, fmt.Errorf("inspect Docker network: %w", err)
	}
	if result.Code != 0 {
		listing, listErr := c.capture(ctx, "network", "ls", "--filter", "name=^"+network+"$", "--format", "{{.Name}}")
		if listErr != nil {
			return nil, false, fmt.Errorf("list Docker networks: %w", listErr)
		}
		if listing.Code != 0 {
			return nil, false, failed("Docker network listing", listing)
		}
		for _, name := range strings.Fields(listing.Stdout) {
			if name == network {
				return nil, false, failed("Docker network inspection", result)
			}
		}
		return nil, false, nil
	}
	labels := map[string]string{}
	trimmed := strings.TrimSpace(result.Stdout)
	if trimmed != "null" && trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &labels); err != nil {
			return nil, false, errors.New("Docker returned unreadable network labels")
		}
	}
	return labels, true, nil
}

func (c *Client) Acquire(ctx context.Context, selected, container string) (Lease, error) {
	if !validName(selected) || !validName(container) {
		return Lease{}, errors.New("the Docker network selection is invalid")
	}
	if selected != "bridge" && selected != buildinfo.ManagedNetwork {
		return Lease{Name: selected, Container: container}, nil
	}
	labels, exists, err := c.networkLabels(ctx, buildinfo.ManagedNetwork)
	if err != nil {
		return Lease{}, err
	}
	if exists && labels[buildinfo.NetworkLabel] != "true" {
		return Lease{}, fmt.Errorf("Docker network %s exists but is not managed by UnAPI'd", buildinfo.ManagedNetwork)
	}
	if selected == buildinfo.ManagedNetwork {
		if !exists {
			return Lease{}, fmt.Errorf("Docker network %s disappeared during setup", buildinfo.ManagedNetwork)
		}
		return Lease{Name: buildinfo.ManagedNetwork, Container: container}, nil
	}

	lease := Lease{Name: buildinfo.ManagedNetwork, Container: container}
	if !exists {
		result, runErr := c.capture(ctx, "network", "create", "--driver", "bridge", "--label", buildinfo.NetworkLabel+"=true", buildinfo.ManagedNetwork)
		if runErr != nil {
			return Lease{}, fmt.Errorf("create private Docker network: %w", runErr)
		}
		if result.Code != 0 {
			return Lease{}, failed("Private Docker network creation", result)
		}
		lease.Created = true
	}
	attached, err := c.Networks(ctx, container)
	if err != nil {
		_ = c.Rollback(ctx, lease)
		return Lease{}, err
	}
	for _, name := range attached {
		if name == buildinfo.ManagedNetwork {
			return lease, nil
		}
	}
	result, runErr := c.capture(ctx, "network", "connect", buildinfo.ManagedNetwork, container)
	if runErr != nil {
		_ = c.Rollback(ctx, lease)
		return Lease{}, fmt.Errorf("attach n8n to private Docker network: %w", runErr)
	}
	if result.Code != 0 {
		_ = c.Rollback(ctx, lease)
		return Lease{}, failed("n8n private-network attachment", result)
	}
	lease.Connected = true
	return lease, nil
}

func (c *Client) Rollback(ctx context.Context, lease Lease) error {
	var problems []string
	if lease.Connected {
		result, err := c.capture(ctx, "network", "disconnect", buildinfo.ManagedNetwork, lease.Container)
		if err != nil || result.Code != 0 {
			problems = append(problems, "disconnect n8n")
		}
	}
	if lease.Created {
		result, err := c.capture(ctx, "network", "rm", buildinfo.ManagedNetwork)
		if err != nil || result.Code != 0 {
			problems = append(problems, "remove private network")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("network rollback could not %s", strings.Join(problems, " or "))
	}
	return nil
}

func (c *Client) Compose(ctx context.Context, file, project string, args ...string) (process.Result, error) {
	base := []string{"compose", "--project-name", project, "--file", file}
	return c.capture(ctx, append(base, args...)...)
}

func ParseServiceStates(output string) ([]ServiceState, error) {
	var states []ServiceState
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			var batch []ServiceState
			if err := json.Unmarshal([]byte(line), &batch); err != nil {
				return nil, errors.New("Docker Compose returned unreadable service state")
			}
			states = append(states, batch...)
			continue
		}
		var state ServiceState
		if err := json.Unmarshal([]byte(line), &state); err != nil {
			return nil, errors.New("Docker Compose returned unreadable service state")
		}
		states = append(states, state)
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.New("Docker Compose returned unreadable service state")
	}
	return states, nil
}

func InternalOnly(states []ServiceState) bool {
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		for _, publisher := range state.Publishers {
			if publisher.PublishedPort > 0 || strings.TrimSpace(publisher.URL) != "" {
				return false
			}
		}
	}
	return true
}
