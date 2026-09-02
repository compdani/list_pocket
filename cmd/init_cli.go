package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/compdani/list_pocket/internal/config"
	flag "github.com/spf13/pflag"
)

func initFlags(ko *config.Conf) {
	f := flag.NewFlagSet("config", flag.ContinueOnError)
	f.ParseErrorsWhitelist.UnknownFlags = true
	f.Usage = func() {
		// Register --help handler.
		fmt.Println(f.FlagUsages())
		os.Exit(0)
	}

	// Register the commandline flags.
	f.StringSlice("config", []string{"config.toml"},
		"path to one or more config files (will be merged in order)")
	f.Bool("install", false, "deprecated: migrations run automatically on serve/startup")
	f.Bool("idempotent", false, "deprecated: kept for listmonk automation compatibility")
	f.Bool("upgrade", false, "deprecated: migrations run automatically on serve/startup")
	f.Bool("version", false, "show current version of the build")
	f.Bool("new-config", false, "generate sample config file (at path given in --config)")
	f.String("static-dir", "", "(optional) path to directory with static files")
	f.String("i18n-dir", "", "(optional) path to directory with i18n language files")
	f.Bool("yes", false, "deprecated: kept for listmonk automation compatibility")
	f.Bool("passive", false, "run in passive mode where campaigns are not processed")
	args := os.Args[1:]
	// Strip PocketBase subcommands so listmonk-style flags still parse cleanly.
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "serve", "start", "migrate", "superuser":
			args = args[1:]
		}
	}

	if err := f.Parse(args); err != nil {
		lo.Fatalf("error loading flags: %v", err)
	}

	if err := ko.LoadFlags(f); err != nil {
		lo.Fatalf("error loading config: %v", err)
	}
}

// ensureDefaultServeArgs makes `serve` the default PocketBase command and bridges
// config.toml app.address into PocketBase's --http flag when missing.
// List Pocket–only flags (--config, --passive, …) are stripped so PocketBase's
// cobra parser does not reject them when they appear after a subcommand
// (e.g. `serve --config /app/config.toml` from the Docker CMD).
func ensureDefaultServeArgs() {
	args := stripListPocketFlags(append([]string(nil), os.Args[1:]...))
	for i, a := range args {
		if a == "start" {
			args[i] = "serve"
		}
	}

	httpAddr := strings.TrimSpace(ko.String("app.address"))
	cmdIdx, cmd := findPocketBaseCommand(args)
	switch cmd {
	case "serve":
		if httpAddr != "" && !hasCLIFlag(args, "http") {
			args = insertCLIArgs(args, cmdIdx+1, "--http="+httpAddr)
		}
		os.Args = append([]string{os.Args[0]}, args...)
		return
	case "migrate", "superuser":
		os.Args = append([]string{os.Args[0]}, args...)
		return
	}

	insert := []string{"serve"}
	if httpAddr != "" && !hasCLIFlag(args, "http") {
		insert = append(insert, "--http="+httpAddr)
	}
	os.Args = append([]string{os.Args[0]}, append(insert, args...)...)
}

// listPocketFlagsWithValue are List Pocket flags that take a separate argv value.
// They are parsed by initFlags and must not reach PocketBase's CLI.
var listPocketFlagsWithValue = map[string]bool{
	"--config": true, "--static-dir": true, "--i18n-dir": true,
}

// listPocketBoolFlags are List Pocket boolean flags that must not reach PocketBase.
var listPocketBoolFlags = map[string]bool{
	"--install": true, "--idempotent": true, "--upgrade": true,
	"--version": true, "--new-config": true, "--yes": true, "--passive": true,
}

// flags that take a separate argv value (not --flag=value).
var cliFlagsWithValue = map[string]bool{
	"--config": true, "--static-dir": true, "--i18n-dir": true,
	"--http": true, "--https": true, "--dir": true, "--encryptionEnv": true,
	"--queryTimeout": true, "--origins": true,
	"-c": true,
}

// stripListPocketFlags removes List Pocket–only flags (already consumed by initFlags)
// so PocketBase's cobra root/serve commands do not see unknown flags.
func stripListPocketFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if name, val, ok := strings.Cut(a, "="); ok && strings.HasPrefix(name, "-") {
			if listPocketFlagsWithValue[name] || listPocketBoolFlags[name] {
				_ = val
				continue
			}
			out = append(out, a)
			continue
		}
		if listPocketFlagsWithValue[a] {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++ // skip value
			}
			continue
		}
		if listPocketBoolFlags[a] {
			continue
		}
		out = append(out, a)
	}
	return out
}

func findPocketBaseCommand(args []string) (int, string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if i+1 < len(args) {
				return classifyPBCommand(i+1, args[i+1])
			}
			return -1, ""
		}
		if strings.HasPrefix(a, "-") {
			if strings.Contains(a, "=") {
				continue
			}
			if cliFlagsWithValue[a] {
				i++ // skip value
			}
			continue
		}
		return classifyPBCommand(i, a)
	}
	return -1, ""
}

func classifyPBCommand(idx int, name string) (int, string) {
	switch name {
	case "serve", "migrate", "superuser":
		return idx, name
	default:
		return -1, ""
	}
}

func hasCLIFlag(args []string, name string) bool {
	long := "--" + name
	for _, a := range args {
		if a == long || strings.HasPrefix(a, long+"=") {
			return true
		}
	}
	return false
}

func insertCLIArgs(args []string, idx int, values ...string) []string {
	if idx < 0 {
		idx = 0
	}
	if idx > len(args) {
		idx = len(args)
	}
	out := make([]string, 0, len(args)+len(values))
	out = append(out, args[:idx]...)
	out = append(out, values...)
	out = append(out, args[idx:]...)
	return out
}

// initConfigFiles loads the given config files into the config instance.
func initConfigFiles(files []string, ko *config.Conf) {
	for _, f := range files {
		lo.Printf("reading config: %s", f)
		if err := ko.LoadTOMLFile(f); err != nil {
			if os.IsNotExist(err) {
				lo.Fatal("config file not found. If there isn't one yet, run --new-config to generate one.")
			}
			lo.Fatalf("error loading config from file: %v.", err)
		}
	}
}
