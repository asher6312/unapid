package console

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/asher6312/unapid/internal/account"
	"github.com/asher6312/unapid/internal/buildinfo"
	"github.com/asher6312/unapid/internal/dockerctl"
	"github.com/asher6312/unapid/internal/reconcile"
	"github.com/asher6312/unapid/internal/secret"
)

const help = `UnAPI'd 2.0.0

Usage:
  unapid             Sign in with ChatGPT and configure n8n API access
  unapid setup       Run the same interactive setup
  unapid status      Show local sign-in, n8n, and API runtime status
  unapid --help      Show this help
  unapid --version   Show the version
`

type Program struct {
	docker  *dockerctl.Client
	account *account.Store
	runtime *reconcile.Reconciler
	input   *os.File
	output  io.Writer
}

func New(docker *dockerctl.Client, accountStore *account.Store, runtime *reconcile.Reconciler) *Program {
	return &Program{docker: docker, account: accountStore, runtime: runtime, input: os.Stdin, output: os.Stdout}
}

func banner(output io.Writer) {
	fmt.Fprintln(output)
	fmt.Fprintln(output, buildinfo.Name)
	fmt.Fprintln(output, "------")
	fmt.Fprintln(output, "ChatGPT subscription access for this server's n8n.")
	fmt.Fprintln(output)
}

func AskYesNo(reader *bufio.Reader, output io.Writer, question string, defaultYes bool) (bool, error) {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	for {
		fmt.Fprintf(output, "%s [%s]: ", question, hint)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		switch answer {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if errors.Is(err, io.EOF) {
			return false, io.EOF
		}
	}
}

func ChooseContainer(reader *bufio.Reader, output io.Writer, containers []dockerctl.Container) (dockerctl.Container, error) {
	if len(containers) == 0 {
		return dockerctl.Container{}, errors.New("no running official n8n container was found")
	}
	if len(containers) == 1 {
		return containers[0], nil
	}
	fmt.Fprintln(output, "Choose the n8n container:")
	for index, container := range containers {
		fmt.Fprintf(output, "  %d. %s\n", index+1, container.Name)
	}
	for {
		fmt.Fprint(output, "Choose a number: ")
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return dockerctl.Container{}, err
		}
		answer = strings.TrimSpace(answer)
		index, parseErr := strconv.Atoi(answer)
		if parseErr == nil && index >= 1 && index <= len(containers) {
			return containers[index-1], nil
		}
		if errors.Is(err, io.EOF) {
			return dockerctl.Container{}, io.EOF
		}
	}
}

func (p *Program) Run(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return errors.New("too many arguments; run unapid --help for usage")
	}
	command := ""
	if len(args) == 1 {
		command = args[0]
	}
	switch command {
	case "--help", "-h", "help":
		fmt.Fprint(p.output, help)
		return nil
	case "--version", "-v":
		fmt.Fprintln(p.output, buildinfo.Version)
		return nil
	case "status":
		return p.showStatus(ctx)
	case "", "setup":
		return p.setup(ctx)
	default:
		return fmt.Errorf("unknown command %q; run unapid --help for usage", command)
	}
}

func (p *Program) showStatus(ctx context.Context) error {
	banner(p.output)
	if err := p.docker.Require(ctx); err != nil {
		return err
	}
	containers, err := p.docker.RunningN8N(ctx)
	if err != nil {
		return err
	}
	auth := p.account.Status()
	signIn := "not configured"
	if auth.Valid {
		signIn = "ready"
	} else if auth.Exists {
		signIn = "invalid"
	}
	fmt.Fprintf(p.output, "ChatGPT sign-in: %s\n", signIn)
	if len(containers) == 0 {
		fmt.Fprintln(p.output, "n8n: not running")
	} else {
		names := make([]string, 0, len(containers))
		for _, container := range containers {
			names = append(names, container.Name)
		}
		fmt.Fprintf(p.output, "n8n: %s\n", strings.Join(names, ", "))
	}
	runtime := p.runtime.Status(ctx)
	if !runtime.Installed {
		fmt.Fprintln(p.output, "API runtime: not installed")
		return nil
	}
	state := "stopped"
	if runtime.Running && runtime.Healthy {
		state = "running"
	} else if runtime.Running {
		state = "starting"
	}
	fmt.Fprintf(p.output, "API runtime: %s\n", state)
	if runtime.Safe {
		fmt.Fprintln(p.output, "Host port: not published (safe)")
	} else {
		fmt.Fprintln(p.output, "Host port: unsafe or unknown")
	}
	return nil
}

func (p *Program) setup(ctx context.Context) error {
	if os.Geteuid() != 0 {
		return errors.New("run UnAPI'd setup as root: sudo unapid")
	}
	inputInfo, err := p.input.Stat()
	if err != nil || inputInfo.Mode()&os.ModeCharDevice == 0 {
		return errors.New("UnAPI'd setup needs an interactive terminal")
	}
	banner(p.output)
	fmt.Fprintln(p.output, "Checking Docker and the running n8n container...")
	if err := p.docker.Require(ctx); err != nil {
		return err
	}
	containers, err := p.docker.RunningN8N(ctx)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(p.input)
	container, err := ChooseContainer(reader, p.output, containers)
	if err != nil {
		return err
	}
	networks, err := p.docker.Networks(ctx, container.Name)
	if err != nil {
		return err
	}
	network, err := dockerctl.PickNetwork(networks)
	if err != nil {
		return err
	}

	fmt.Fprintln(p.output)
	var authContents []byte
	replaceAuth := false
	authStatus := p.account.Status()
	if authStatus.Valid {
		reuse, promptErr := AskYesNo(reader, p.output, "Use the existing UnAPI'd ChatGPT sign-in?", true)
		if promptErr != nil {
			return promptErr
		}
		if reuse {
			authContents, err = p.account.Read(true)
			if err != nil {
				return err
			}
		}
	}
	if len(authContents) == 0 {
		fmt.Fprintln(p.output, "UnAPI'd keeps its ChatGPT sign-in separate from ~/.codex and ~/.hermes.")
		signIn, promptErr := AskYesNo(reader, p.output, "Sign in with your ChatGPT subscription now?", true)
		if promptErr != nil {
			return promptErr
		}
		if !signIn {
			fmt.Fprintln(p.output, "Setup cancelled; nothing was changed.")
			return nil
		}
		fmt.Fprintln(p.output)
		authContents, err = p.account.DeviceLogin(ctx)
		if err != nil {
			return err
		}
		replaceAuth = true
	}
	apiKey, err := secret.LoadOrCreate()
	if err != nil {
		return err
	}
	fmt.Fprintln(p.output)
	fmt.Fprintln(p.output, "Configuring and verifying API access...")
	result, err := p.runtime.Apply(ctx, container.Name, network, authContents, apiKey, replaceAuth)
	if err != nil {
		return err
	}
	fmt.Fprintln(p.output)
	fmt.Fprintf(p.output, "UnAPI'd %s. Your n8n container was left running.\n", result.Mode)
	fmt.Fprintf(p.output, "Base URL: %s\n", result.BaseURL)
	fmt.Fprintf(p.output, "API key: %s\n", result.APIKey)
	fmt.Fprintf(p.output, "Models: %s\n", strings.Join(result.Models, ", "))
	return nil
}
