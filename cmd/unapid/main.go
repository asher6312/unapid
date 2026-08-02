package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/asher6312/unapid/internal/account"
	"github.com/asher6312/unapid/internal/console"
	"github.com/asher6312/unapid/internal/dockerctl"
	"github.com/asher6312/unapid/internal/probe"
	"github.com/asher6312/unapid/internal/process"
	"github.com/asher6312/unapid/internal/proxy"
	"github.com/asher6312/unapid/internal/reconcile"
)

func gatewayCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("internal-gateway", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	listen := flags.String("listen", "", "listen address")
	upstream := flags.String("upstream", "", "translator URL")
	keyFile := flags.String("key-file", "", "API key file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *listen == "" || *upstream == "" || *keyFile == "" {
		return errors.New("the internal gateway configuration is incomplete")
	}
	return proxy.Run(ctx, *listen, *upstream, *keyFile)
}

func probeCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("internal-probe", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	endpoint := flags.String("url", "", "probe URL")
	keyFile := flags.String("key-file", "", "API key file")
	printBody := flags.Bool("print", false, "print response")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *endpoint == "" {
		return errors.New("the internal probe configuration is incomplete")
	}
	body, err := probe.Fetch(ctx, *endpoint, *keyFile)
	if err != nil {
		return err
	}
	if *printBody {
		fmt.Print(string(body))
	}
	return nil
}

func run(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "internal-gateway":
			return gatewayCommand(ctx, args[1:])
		case "internal-probe":
			return probeCommand(ctx, args[1:])
		}
	}
	runner := process.Local{}
	docker := dockerctl.New(runner)
	accountStore, err := account.New(runner)
	if err != nil {
		return err
	}
	program := console.New(docker, accountStore, reconcile.New(docker))
	return program.Run(ctx, args)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "UnAPI'd: Aborted with Ctrl+C")
		} else {
			fmt.Fprintf(os.Stderr, "UnAPI'd: %s\n", err)
		}
		os.Exit(1)
	}
}
