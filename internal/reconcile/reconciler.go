package reconcile

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/asher6312/unapid/internal/account"
	"github.com/asher6312/unapid/internal/buildinfo"
	"github.com/asher6312/unapid/internal/dockerctl"
	"github.com/asher6312/unapid/internal/material"
	"github.com/asher6312/unapid/internal/process"
	"github.com/asher6312/unapid/internal/secret"
)

var modelID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)

type Result struct {
	BaseURL string
	APIKey  string
	Models  []string
	Mode    string
}

type RuntimeStatus struct {
	Installed bool
	Running   bool
	Healthy   bool
	Safe      bool
}

type Reconciler struct {
	docker *dockerctl.Client
}

func New(docker *dockerctl.Client) *Reconciler {
	return &Reconciler{docker: docker}
}

func validOwner(path string) bool {
	contents, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var owner struct {
		Owner  string `json:"owner"`
		Schema int    `json:"schema"`
	}
	return json.Unmarshal(contents, &owner) == nil && owner.Owner == "unapid" && owner.Schema == 2
}

func validateState() (string, error) {
	auth, err := os.ReadFile(filepath.Join(buildinfo.StateRoot, "codex", "auth.json"))
	if err != nil || account.Validate(auth) != nil {
		return "", errors.New("the persistent ChatGPT credential file is invalid")
	}
	contents, err := os.ReadFile(filepath.Join(buildinfo.StateRoot, "api-key"))
	if err != nil {
		return "", errors.New("the persistent gateway API key could not be read")
	}
	return secret.Validate(string(contents))
}

func writeStateCredentials(root string, auth []byte, apiKey string) error {
	codexDirectory := filepath.Join(root, "codex")
	if err := os.MkdirAll(codexDirectory, 0o700); err != nil {
		return fmt.Errorf("create persistent credential directory: %w", err)
	}
	if err := os.Chmod(codexDirectory, 0o700); err != nil {
		return fmt.Errorf("secure persistent credential directory: %w", err)
	}
	if err := writeFile(filepath.Join(codexDirectory, "auth.json"), auth, 0o600); err != nil {
		return fmt.Errorf("write persistent ChatGPT credentials: %w", err)
	}
	if err := writeFile(filepath.Join(root, "api-key"), []byte(apiKey+"\n"), 0o600); err != nil {
		return fmt.Errorf("write persistent gateway API key: %w", err)
	}
	for _, path := range []string{root, codexDirectory, filepath.Join(codexDirectory, "auth.json"), filepath.Join(root, "api-key")} {
		if err := os.Chown(path, 1000, 1000); err != nil {
			return fmt.Errorf("set persistent credential ownership: %w", err)
		}
	}
	return nil
}

func replaceStateAuth(auth []byte) error {
	directory := filepath.Join(buildinfo.StateRoot, "codex")
	temporary, err := os.CreateTemp(directory, ".auth-update-")
	if err != nil {
		return fmt.Errorf("stage ChatGPT credential update: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := true
	defer func() {
		_ = temporary.Close()
		if cleanup {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if err := temporary.Chown(1000, 1000); err != nil {
		return err
	}
	if _, err := temporary.Write(auth); err != nil {
		return fmt.Errorf("write ChatGPT credential update: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync ChatGPT credential update: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close ChatGPT credential update: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "auth.json")); err != nil {
		return fmt.Errorf("activate ChatGPT credential update: %w", err)
	}
	cleanup = false
	return nil
}

func replaceStateAuthWithRollback(auth []byte) (func() error, error) {
	currentPath := filepath.Join(buildinfo.StateRoot, "codex", "auth.json")
	previous, err := os.ReadFile(currentPath)
	if err != nil || account.Validate(previous) != nil {
		return nil, errors.New("the existing ChatGPT credential file is invalid")
	}
	if bytes.Equal(previous, auth) {
		return nil, nil
	}
	if err := replaceStateAuth(auth); err != nil {
		return nil, err
	}
	return func() error { return replaceStateAuth(previous) }, nil
}

func createState(auth []byte, apiKey string) error {
	stage, err := os.MkdirTemp(filepath.Dir(buildinfo.StateRoot), ".state-stage-")
	if err != nil {
		return fmt.Errorf("create persistent-state stage: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0o700); err != nil {
		return err
	}
	if err := writeStateCredentials(stage, auth, apiKey); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(stage, "owner.json"), material.OwnerDocument(), 0o644); err != nil {
		return fmt.Errorf("write persistent-state owner file: %w", err)
	}
	if err := os.Rename(stage, buildinfo.StateRoot); err != nil {
		return fmt.Errorf("activate persistent state: %w", err)
	}
	cleanup = false
	return nil
}

func ensureState(auth []byte, apiKey string, replaceAuth bool) (string, func() error, error) {
	if err := os.MkdirAll(filepath.Dir(buildinfo.StateRoot), 0o755); err != nil {
		return "", nil, fmt.Errorf("create UnAPI'd data directory: %w", err)
	}
	if info, err := os.Stat(buildinfo.StateRoot); err == nil {
		if !info.IsDir() || !validOwner(buildinfo.StateOwnerFile) {
			return "", nil, errors.New("the UnAPI'd state directory exists but is not owned by UnAPI'd")
		}
		effectiveKey, err := validateState()
		if err != nil {
			return "", nil, err
		}
		if !replaceAuth {
			return effectiveKey, nil, nil
		}
		rollback, err := replaceStateAuthWithRollback(auth)
		return effectiveKey, rollback, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("inspect persistent state: %w", err)
	}
	if err := createState(auth, apiKey); err != nil {
		return "", nil, err
	}
	return apiKey, nil, nil
}

func writeFile(path string, contents []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, contents, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func copyExecutable(destination string) error {
	sourcePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve UnAPI'd executable: %w", err)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open UnAPI'd executable: %w", err)
	}
	defer source.Close()
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return fmt.Errorf("create runtime executable: %w", err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return fmt.Errorf("copy runtime executable: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close runtime executable: %w", closeErr)
	}
	return os.Chmod(destination, 0o755)
}

func prepare(network string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(buildinfo.RuntimeRoot), 0o755); err != nil {
		return "", fmt.Errorf("create UnAPI'd data directory: %w", err)
	}
	stage, err := os.MkdirTemp(filepath.Dir(buildinfo.RuntimeRoot), ".runtime-stage-")
	if err != nil {
		return "", fmt.Errorf("create deployment stage: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := os.Chmod(stage, 0o755); err != nil {
		return "", err
	}
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{
		{filepath.Join(stage, "bin"), 0o755},
	} {
		if err := os.MkdirAll(directory.path, directory.mode); err != nil {
			return "", fmt.Errorf("create runtime directory: %w", err)
		}
		if err := os.Chmod(directory.path, directory.mode); err != nil {
			return "", fmt.Errorf("secure runtime directory: %w", err)
		}
	}
	files, err := material.Files(network)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if err := writeFile(filepath.Join(stage, file.Name), file.Data, os.FileMode(file.Mode)); err != nil {
			return "", fmt.Errorf("write runtime file %s: %w", file.Name, err)
		}
	}
	if err := copyExecutable(filepath.Join(stage, "bin", "unapid")); err != nil {
		return "", err
	}
	cleanup = false
	return stage, nil
}

func activate(stage string) (string, error) {
	backup := ""
	if info, err := os.Stat(buildinfo.RuntimeRoot); err == nil {
		if !info.IsDir() || !validOwner(buildinfo.OwnerFile) {
			return "", errors.New("the UnAPI'd runtime directory exists but is not owned by UnAPI'd")
		}
		backup = filepath.Join(filepath.Dir(buildinfo.RuntimeRoot), fmt.Sprintf(".runtime-backup-%d", time.Now().UnixNano()))
		if err := os.Rename(buildinfo.RuntimeRoot, backup); err != nil {
			return "", fmt.Errorf("stage existing runtime: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect runtime directory: %w", err)
	}
	if err := os.Rename(stage, buildinfo.RuntimeRoot); err != nil {
		if backup != "" {
			_ = os.Rename(backup, buildinfo.RuntimeRoot)
		}
		return "", fmt.Errorf("activate runtime: %w", err)
	}
	return backup, nil
}

func commandError(label string, result process.Result, err error) error {
	if err != nil {
		return fmt.Errorf("%s failed: %w", label, err)
	}
	if result.Code == 0 {
		return nil
	}
	detail := process.Detail(result)
	if detail == "" {
		return fmt.Errorf("%s failed", label)
	}
	return fmt.Errorf("%s failed: %s", label, detail)
}

func (r *Reconciler) compose(ctx context.Context, args ...string) (process.Result, error) {
	return r.docker.Compose(ctx, filepath.Join(buildinfo.RuntimeRoot, "compose.yaml"), buildinfo.Project, args...)
}

func (r *Reconciler) deployCommands(ctx context.Context) error {
	commands := []struct {
		label string
		args  []string
	}{
		{"Compose validation", []string{"config", "--quiet"}},
		{"runtime image build", []string{"build"}},
		{"runtime startup", []string{"up", "-d", "--wait", "--wait-timeout", "90"}},
	}
	for _, command := range commands {
		result, err := r.compose(ctx, command.args...)
		if problem := commandError(command.label, result, err); problem != nil {
			return problem
		}
	}
	return nil
}

func parseModels(contents []byte) ([]string, error) {
	var document struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(contents, &document); err != nil {
		return nil, errors.New("the model response is invalid")
	}
	seen := map[string]bool{}
	var models []string
	for _, item := range document.Data {
		if modelID.MatchString(item.ID) && !seen[item.ID] {
			seen[item.ID] = true
			models = append(models, item.ID)
		}
	}
	if len(models) == 0 {
		return nil, errors.New("the model response contains no usable models")
	}
	sort.Strings(models)
	return models, nil
}

func (r *Reconciler) verify(ctx context.Context) ([]string, error) {
	stateResult, err := r.compose(ctx, "ps", "--format", "json")
	if problem := commandError("runtime status check", stateResult, err); problem != nil {
		return nil, problem
	}
	states, err := dockerctl.ParseServiceStates(stateResult.Stdout)
	if err != nil {
		return nil, err
	}
	expected := map[string]bool{buildinfo.APIService: false, buildinfo.OAuthService: false}
	for _, state := range states {
		if _, relevant := expected[state.Service]; relevant && state.State == "running" && state.Health == "healthy" {
			expected[state.Service] = true
		}
	}
	if !expected[buildinfo.APIService] || !expected[buildinfo.OAuthService] {
		return nil, errors.New("the UnAPI'd runtime did not reach a healthy state")
	}
	if !dockerctl.InternalOnly(states) {
		return nil, errors.New("safety check failed: the UnAPI'd runtime published a host port")
	}
	probeResult, probeErr := r.compose(ctx,
		"exec", "-T", buildinfo.APIService,
		"/unapid", "internal-probe",
		"--url", fmt.Sprintf("http://127.0.0.1:%d/v1/models", buildinfo.GatewayPort),
		"--key-file", "/run/secrets/unapid_api_key",
		"--print",
	)
	if problem := commandError("authenticated model check", probeResult, probeErr); problem != nil {
		return nil, problem
	}
	return parseModels([]byte(probeResult.Stdout))
}

func (r *Reconciler) restore(ctx context.Context, backup string, rollbackState func() error) {
	_, _ = r.compose(ctx, "down", "--remove-orphans")
	_ = os.RemoveAll(buildinfo.RuntimeRoot)
	if rollbackState != nil {
		_ = rollbackState()
	}
	if backup != "" {
		if os.Rename(backup, buildinfo.RuntimeRoot) == nil {
			_, _ = r.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "90")
		}
	}
}

func (r *Reconciler) Apply(ctx context.Context, container, network string, auth []byte, apiKey string, replaceAuth bool) (Result, error) {
	if err := account.Validate(auth); err != nil {
		return Result{}, err
	}
	apiKey, err := secret.Validate(apiKey)
	if err != nil {
		return Result{}, err
	}
	mode := "installed"
	if validOwner(buildinfo.OwnerFile) {
		mode = "updated"
	}
	lease, err := r.docker.Acquire(ctx, network, container)
	if err != nil {
		return Result{}, err
	}
	apiKey, rollbackState, err := ensureState(auth, apiKey, replaceAuth)
	if err != nil {
		_ = r.docker.Rollback(context.Background(), lease)
		return Result{}, err
	}
	stage, err := prepare(lease.Name)
	if err != nil {
		if rollbackState != nil {
			_ = rollbackState()
		}
		_ = r.docker.Rollback(context.Background(), lease)
		return Result{}, err
	}
	backup, err := activate(stage)
	if err != nil {
		_ = os.RemoveAll(stage)
		if rollbackState != nil {
			_ = rollbackState()
		}
		_ = r.docker.Rollback(context.Background(), lease)
		return Result{}, err
	}
	models, err := func() ([]string, error) {
		if err := r.deployCommands(ctx); err != nil {
			return nil, err
		}
		return r.verify(ctx)
	}()
	if err != nil {
		r.restore(context.Background(), backup, rollbackState)
		_ = r.docker.Rollback(context.Background(), lease)
		return Result{}, fmt.Errorf("runtime deployment failed; n8n was not restarted: %w", err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return Result{
		BaseURL: fmt.Sprintf("http://%s:%d/v1", buildinfo.GatewayHost, buildinfo.GatewayPort),
		APIKey:  apiKey,
		Models:  models,
		Mode:    mode,
	}, nil
}

func (r *Reconciler) Status(ctx context.Context) RuntimeStatus {
	if !validOwner(buildinfo.OwnerFile) {
		return RuntimeStatus{}
	}
	result, err := r.compose(ctx, "ps", "--format", "json")
	if err != nil || result.Code != 0 {
		return RuntimeStatus{Installed: true}
	}
	states, err := dockerctl.ParseServiceStates(result.Stdout)
	if err != nil || len(states) == 0 {
		return RuntimeStatus{Installed: true}
	}
	status := RuntimeStatus{Installed: true, Running: true, Healthy: true, Safe: dockerctl.InternalOnly(states)}
	for _, state := range states {
		if state.State != "running" {
			status.Running = false
		}
		if state.Health != "healthy" {
			status.Healthy = false
		}
	}
	return status
}

func SensitivePaths() []string {
	return []string{
		filepath.Join(buildinfo.StateRoot, "codex", "auth.json"),
		filepath.Join(buildinfo.StateRoot, "api-key"),
	}
}

func ParseModelResponse(contents string) ([]string, error) {
	return parseModels([]byte(strings.TrimSpace(contents)))
}
