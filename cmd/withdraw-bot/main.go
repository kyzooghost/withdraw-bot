package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"withdraw-bot/internal/app"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:]))
}

func run(ctx context.Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: withdraw-bot <monitor|bootstrap|config-check> --config <path>")
		return 2
	}

	mode, ok := app.ParseMode(args[0])
	if !ok {
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", args[0])
		return 2
	}

	fs := flag.NewFlagSet(string(mode), flag.ContinueOnError)
	configPath := fs.String("config", "config/config.example.yaml", "path to YAML config")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	if err := app.Run(ctx, mode, *configPath); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}
