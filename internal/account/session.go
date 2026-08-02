package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/asher6312/unapid/internal/buildinfo"
	"github.com/asher6312/unapid/internal/process"
)

const maxCredentialBytes = 128 * 1024

var sudoName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

type Status struct {
	Exists    bool
	Valid     bool
	Path      string
	UpdatedAt time.Time
}

type Store struct {
	runner process.Runner
	home   string
	env    []string
}

func New(runner process.Runner) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return nil, errors.New("the current home directory could not be resolved")
	}
	return &Store{runner: runner, home: home, env: os.Environ()}, nil
}

func NewWith(runner process.Runner, home string, env []string) *Store {
	return &Store{runner: runner, home: home, env: append([]string(nil), env...)}
}

func (s *Store) isolatedHome() string {
	return filepath.Join(s.home, ".unapid", "codex")
}

func Validate(contents []byte) error {
	if len(contents) == 0 || len(contents) > maxCredentialBytes {
		return errors.New("the UnAPI'd ChatGPT credential file is invalid")
	}
	var document struct {
		Mode   string `json:"auth_mode"`
		Tokens *struct {
			ID      string `json:"id_token"`
			Access  string `json:"access_token"`
			Refresh string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(contents, &document); err != nil || document.Mode != "chatgpt" || document.Tokens == nil || document.Tokens.ID == "" || document.Tokens.Access == "" || document.Tokens.Refresh == "" {
		return errors.New("the UnAPI'd ChatGPT credential file is invalid")
	}
	return nil
}

func ownerMatches(path string) bool {
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

func (s *Store) candidates(includeDeployed bool) []string {
	var paths []string
	if includeDeployed && ownerMatches(buildinfo.StateOwnerFile) {
		paths = append(paths, filepath.Join(buildinfo.StateRoot, "codex", "auth.json"))
	}
	paths = append(paths, filepath.Join(s.isolatedHome(), "auth.json"))
	return paths
}

func (s *Store) Read(includeDeployed bool) ([]byte, error) {
	for _, path := range s.candidates(includeDeployed) {
		contents, err := os.ReadFile(path)
		if err == nil && Validate(contents) == nil {
			return contents, nil
		}
	}
	return nil, errors.New("no valid UnAPI'd ChatGPT sign-in was found")
}

func (s *Store) Status() Status {
	var invalid string
	for _, path := range s.candidates(true) {
		contents, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) && invalid == "" {
				invalid = path
			}
			continue
		}
		if Validate(contents) != nil {
			if invalid == "" {
				invalid = path
			}
			continue
		}
		status := Status{Exists: true, Valid: true, Path: path}
		if info, statErr := os.Stat(path); statErr == nil {
			status.UpdatedAt = info.ModTime()
		}
		return status
	}
	if invalid != "" {
		return Status{Exists: true, Path: invalid}
	}
	return Status{Path: filepath.Join(s.isolatedHome(), "auth.json")}
}

type executable struct {
	command string
	binDir  string
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode().Perm()&0o111 != 0
}

func parseVersion(name string) ([3]int, bool) {
	trimmed := strings.TrimPrefix(name, "v")
	trimmed = strings.SplitN(trimmed, "-", 2)[0]
	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var version [3]int
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return [3]int{}, false
		}
		version[index] = value
	}
	return version, true
}

func newerVersion(left, right string) bool {
	a, aOK := parseVersion(left)
	b, bOK := parseVersion(right)
	if !aOK || !bOK {
		return left > right
	}
	for index := range a {
		if a[index] != b[index] {
			return a[index] > b[index]
		}
	}
	return false
}

func sudoHome(env []string) string {
	name := envValue(env, "SUDO_USER")
	uid := envValue(env, "SUDO_UID")
	if name == "" || name == "root" || !sudoName.MatchString(name) || uid == "" {
		return ""
	}
	record, err := user.Lookup(name)
	if err != nil || record.Uid != uid || !filepath.IsAbs(record.HomeDir) {
		return ""
	}
	return filepath.Clean(record.HomeDir)
}

func resolveCodex(env []string) executable {
	for _, directory := range filepath.SplitList(envValue(env, "PATH")) {
		if !filepath.IsAbs(directory) {
			continue
		}
		candidate := filepath.Join(directory, "codex")
		if executableFile(candidate) {
			return executable{command: candidate, binDir: directory}
		}
	}
	home := sudoHome(env)
	if home != "" {
		nvmRoot := filepath.Join(home, ".nvm", "versions", "node")
		entries, _ := os.ReadDir(nvmRoot)
		var versions []string
		for _, entry := range entries {
			if entry.IsDir() {
				if _, valid := parseVersion(entry.Name()); valid {
					versions = append(versions, entry.Name())
				}
			}
		}
		sort.Slice(versions, func(i, j int) bool { return newerVersion(versions[i], versions[j]) })
		for _, version := range versions {
			binDir := filepath.Join(nvmRoot, version, "bin")
			candidate := filepath.Join(binDir, "codex")
			if executableFile(candidate) && executableFile(filepath.Join(binDir, "node")) {
				return executable{command: candidate, binDir: binDir}
			}
		}
		for _, relative := range []string{
			filepath.Join(".volta", "bin", "codex"),
			filepath.Join(".local", "bin", "codex"),
			filepath.Join(".npm-global", "bin", "codex"),
		} {
			candidate := filepath.Join(home, relative)
			if executableFile(candidate) {
				return executable{command: candidate, binDir: filepath.Dir(candidate)}
			}
		}
	}
	return executable{command: "codex"}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for index := len(env) - 1; index >= 0; index-- {
		if strings.HasPrefix(env[index], prefix) {
			return strings.TrimPrefix(env[index], prefix)
		}
	}
	return ""
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		if !strings.HasPrefix(item, prefix) {
			result = append(result, item)
		}
	}
	return append(result, prefix+value)
}

func (s *Store) DeviceLogin(ctx context.Context) ([]byte, error) {
	codexHome := s.isolatedHome()
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		return nil, fmt.Errorf("create isolated sign-in directory: %w", err)
	}
	if err := os.Chmod(codexHome, 0o700); err != nil {
		return nil, fmt.Errorf("secure isolated sign-in directory: %w", err)
	}
	executable := resolveCodex(s.env)
	childEnv := setEnv(s.env, "CODEX_HOME", codexHome)
	if executable.binDir != "" {
		path := envValue(childEnv, "PATH")
		if path == "" {
			path = executable.binDir
		} else {
			path = executable.binDir + string(os.PathListSeparator) + path
		}
		childEnv = setEnv(childEnv, "PATH", path)
	}
	if err := s.runner.Interactive(ctx, childEnv, executable.command, "login", "--device-auth", "-c", `cli_auth_credentials_store="file"`); err != nil {
		return nil, errors.New("Codex CLI is required and ChatGPT device sign-in must finish successfully")
	}
	contents, err := s.Read(false)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Join(codexHome, "auth.json"), 0o600); err != nil {
		return nil, fmt.Errorf("secure ChatGPT credential file: %w", err)
	}
	return contents, nil
}
