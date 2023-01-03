package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"golang.org/x/exp/slices"

	"github.com/guardian/devx-config/log"
	"github.com/guardian/devx-config/store"
)

type Cmd struct {
	Name        string
	Description string
	Run         func(args []string)
}

// Config is common to all subcommands.
type Config struct {
	Service store.Service
	Logger  log.Logger
	Store   store.Store
}

func main() {
	cmds := []Cmd{
		{
			Name:        "get",
			Description: "Gets specific config for service.",
			Run: func(args []string) {
				var name string
				config, parseErr := parseFlags("get", args, false, func(fs *flag.FlagSet) []string {
					required := []string{"name"}

					fs.StringVar(&name, "name", "", "Name of the config item to retrieve.")

					return required
				})

				check(config.Logger, parseErr, "unable to parse config", 2)

				item, err := config.Store.Get(config.Service, name)
				check(config.Logger, err, fmt.Sprintf("unable to get %s for service '%s'", name, config.Service.Prefix()), 1)

				config.Logger.Infof(item.String())
			},
		},
		{
			Name:        "list",
			Description: "List all config for a service.",
			Run: func(args []string) {
				config, parseErr := parseFlags("list", args, false)
				check(config.Logger, parseErr, "", 2)

				items, err := config.Store.List(config.Service)
				check(config.Logger, err, fmt.Sprintf("unable to list for service '%s'", config.Service.Prefix()), 1)

				for _, item := range items {
					config.Logger.Infof(item.String())
				}
			},
		},
		{
			Name:        "set",
			Description: "Sets specific config for a service.",
			Run: func(args []string) {
				var name, value string
				config, parseErr := parseFlags("set", args, false, func(fs *flag.FlagSet) []string {
					required := []string{"name", "value"}

					fs.StringVar(&name, "name", "", "Name of the config item to set.")
					fs.StringVar(&value, "value", "", "Value of the config item to set.")

					return required
				})

				check(config.Logger, parseErr, "", 2)

				isSecret := askYesNo("Is this parameter a secret?")

				err := config.Store.Set(config.Service, name, value, isSecret)
				check(config.Logger, err, fmt.Sprintf("unable to set '%s=%s' for service '%s'", name, value, config.Service.Prefix()), 1)
			},
		},
		{
			Name:        "delete",
			Description: "Deletes specific config for a service.",
			Run: func(args []string) {
				var name string
				config, parseErr := parseFlags("delete", args, false, func(fs *flag.FlagSet) []string {
					required := []string{"name"}

					fs.StringVar(&name, "name", "", "Name of the config item to delete.")

					return required
				})

				check(config.Logger, parseErr, "", 2)

				ok := askYesNo(fmt.Sprintf("Are you sure you want to delete '%s'?", name))
				if !ok {
					config.Logger.Infof("Config item '%s' has not been deleted.", name)
					return
				}

				err := config.Store.Delete(config.Service, name)
				check(config.Logger, err, fmt.Sprintf("unable to delete '%s' for service '%s'", name, config.Service.Prefix()), 1)
			},
		},
		{
			Name:        "set-local-config",
			Description: "Creates a local .devx-config file to avoid having to type the stack/stage/app args every time.",
			Run: func(args []string) {
				logger := log.New(false)

				config, parseErr := parseFlags("set-local-config", args, true)

				if parseErr != nil {
					app := ask("App: ")
					stack := ask("Stack: ")
					stage := ask("Stage: ")

					writeConfigFile(store.Service{App: app, Stack: stack, Stage: stage}, logger)
					return
				}

				writeConfigFile(config.Service, logger)
			},
		},
	}

	// If no arguments passed display help and return
	argsLength := len(os.Args)
	if argsLength <= 1 {
		printHelp(cmds)
		return
	}

	subcommand := os.Args[1]
	for _, cmd := range cmds {
		if cmd.Name == subcommand {
			cmd.Run(os.Args[2:])
			return
		}
	}

	if strings.Contains(subcommand, "-h") {
		printHelp(cmds)
		return
	}

	logger := log.New(false)
	logger.Infof("unknown subcommand %s", subcommand)
	os.Exit(1)
}

func writeConfigFile(service store.Service, logger log.Logger) {
	out, err := json.Marshal(service)
	check(logger, err, "unable to write JSON", 1)

	err = os.WriteFile(".devx-config", out, 0644)
	check(logger, err, "unable to write .devx-config file", 1)
}

// Parses flags (including the common ones) and returns a Config object. Note,
// also attempts to read service tags from "/etc/config/tags.json", which is
// where cdk-base writes to.
//
// Use 'extras' arg(s) to include additional flags.
func parseFlags(cmd string, args []string, local bool, extras ...func(fs *flag.FlagSet) []string) (Config, error) {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)

	app := fs.String("app", "", "App for your service.")
	stage := fs.String("stage", "", "Stage for your service (typically 'CODE' or 'PROD'.")
	stack := fs.String("stack", "", "Stack for your service.")

	debug := fs.Bool("debug", false, "Whether to enable debug logs.")
	profile := fs.String("profile", "", "Profile for AWS credentials (if running locally).")

	requiredFlags := make([]string, 0)

	for _, extra := range extras {
		req := extra(fs)
		requiredFlags = append(requiredFlags, req...)
	}

	fs.Parse(args)

	logger := log.New(*debug)

	// If running in EC2 with cdk-base, we expect this info to be present so let's use it.
	service, ok := readFileConfig()

	// Else we check the flags are present and use them instead, except if explicitly local
	if !ok || local {
		service = store.Service{Stack: *stack, Stage: *stage, App: *app}
		requiredFlags = append([]string{"app", "stage", "stack"}, requiredFlags...)
	}

	errs := make([]error, 0)
	fs.VisitAll(func(f *flag.Flag) {
		if slices.Contains(requiredFlags, f.Name) {
			if f.Value.String() == "" {
				err := errors.New(fmt.Sprintf("mandatory flag '%s' is not set or is empty\n", f.Name))
				errs = append(errs, err)
			}
		}
	})

	if len(errs) > 0 {
		return Config{}, errs[0]
	}

	store := store.NewSSM(logger, ssmClient(context.TODO(), logger, *profile))

	return Config{Service: service, Logger: logger, Store: store}, nil
}

func readFileConfig() (store.Service, bool) {
	var service store.Service

	data, err := os.ReadFile("/etc/config/tags.json")
	if err != nil {
		data, err = os.ReadFile(".devx-config")
		if err != nil {
			return service, false
		}
	}

	err = json.Unmarshal(data, &service)
	if err != nil {
		return service, false
	}

	return service, true
}

func printHelp(cmds []Cmd) {
	fmt.Println("usage: devx-config [-h] <command> [<args>]")
	fmt.Println()
	for _, cmd := range cmds {
		fmt.Printf("\t%s\t%s\n", cmd.Name, cmd.Description)
	}
	fmt.Println()
	fmt.Println("For help on a specific subcommand, use (e.g.): devx-config get -h.")
}

func ask(question string) string {
	fmt.Print(question)

	got := ""
	fmt.Scanln(&got)

	return got
}

func askYesNo(question string) bool {
	got := ask(question + "(y/n) ")

	switch got {
	case "y":
		return true
	case "n":
		return false
	default:
		fmt.Println("Response must be one of 'y', 'n'.")
		return askYesNo(question)
	}
}

func ssmClient(ctx context.Context, logger log.Logger, profile string) *ssm.Client {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile), config.WithRegion("eu-west-1"))
	check(logger, err, "unable to load default config", 1)
	return ssm.NewFromConfig(cfg)
}

func check(logger log.Logger, err error, msg string, exitCode int) {
	if err != nil {
		logger.Infof("%s; %v", msg, err)
		os.Exit(exitCode)
	}
}
