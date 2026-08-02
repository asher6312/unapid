package material

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"text/template"

	"github.com/asher6312/unapid/internal/buildinfo"
)

//go:embed runtime/*
var assets embed.FS

var dockerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type File struct {
	Name string
	Data []byte
	Mode uint32
}

type composeValues struct {
	Network      string
	GatewayHost  string
	GatewayPort  int
	OAuthPort    int
	OwnerLabel   string
	APIService   string
	OAuthService string
}

func OwnerDocument() []byte {
	document, _ := json.MarshalIndent(map[string]any{
		"owner":   "unapid",
		"schema":  2,
		"version": buildinfo.Version,
	}, "", "  ")
	return append(document, '\n')
}

func Files(network string) ([]File, error) {
	if !dockerName.MatchString(network) {
		return nil, errors.New("the Docker network name is invalid")
	}
	staticFiles := []struct {
		source string
		target string
	}{
		{"runtime/api.Containerfile", "api.Containerfile"},
		{"runtime/translator.Containerfile", "translator.Containerfile"},
		{"runtime/dockerignore", ".dockerignore"},
	}
	files := make([]File, 0, len(staticFiles)+2)
	for _, item := range staticFiles {
		contents, err := assets.ReadFile(item.source)
		if err != nil {
			return nil, fmt.Errorf("read embedded %s: %w", item.source, err)
		}
		files = append(files, File{Name: item.target, Data: contents, Mode: 0o644})
	}
	composeSource, err := assets.ReadFile("runtime/compose.yaml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read embedded Compose template: %w", err)
	}
	tmpl, err := template.New("compose").Option("missingkey=error").Parse(string(composeSource))
	if err != nil {
		return nil, fmt.Errorf("parse embedded Compose template: %w", err)
	}
	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, composeValues{
		Network:      network,
		GatewayHost:  buildinfo.GatewayHost,
		GatewayPort:  buildinfo.GatewayPort,
		OAuthPort:    buildinfo.OAuthPort,
		OwnerLabel:   buildinfo.OwnerLabel,
		APIService:   buildinfo.APIService,
		OAuthService: buildinfo.OAuthService,
	}); err != nil {
		return nil, fmt.Errorf("render Compose configuration: %w", err)
	}
	files = append(files,
		File{Name: "compose.yaml", Data: rendered.Bytes(), Mode: 0o644},
		File{Name: "owner.json", Data: OwnerDocument(), Mode: 0o644},
	)
	return files, nil
}
