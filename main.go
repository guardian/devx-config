package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/spf13/cobra"

	"github.com/guardian/devx-config/config"
	"github.com/guardian/devx-config/log"
	"github.com/guardian/devx-config/store"
)

type Cmd struct {
	Name        string
	Description string
	Run         func(args []string)
}

const (
	InternalError = 1
	InvalidArgs   = 2
)

func main() {
	logger := log.New(readBoolFlag(os.Args[1:], "debug", "Whether to enable debug logs."))

	rootCmd := &cobra.Command{Use: "app"}
	app := rootCmd.PersistentFlags().String("app", "", "App for your service.")
	stack := rootCmd.PersistentFlags().String("stack", "", "Stack for your service.")
	stage := rootCmd.PersistentFlags().String("stage", "", "Stage for your service.")
	profile := rootCmd.PersistentFlags().String("profile", "", "Janus profile for your service (when running locally).")

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get parameter for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to retrieve")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
			conf, err := config.Read(argConf, config.DefaultFiles()...)
			check(logger, err, "Unable to read config", InvalidArgs)

			ssm := store.NewSSM(logger, ssmClient(context.TODO(), logger, *profile))

			service := store.Service{App: conf.App, Stack: conf.Stack, Stage: conf.Stage}
			item, err := ssm.Get(service, *name)
			check(logger, err, fmt.Sprintf("unable to get %s for service '%s'", *name, service.Prefix()), 1)

			logger.Infof(item.String())
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all parameters for a service",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(args)

			argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
			conf, err := config.Read(argConf, config.DefaultFiles()...)
			check(logger, err, "Unable to read config", InvalidArgs)

			ssm := store.NewSSM(logger, ssmClient(context.TODO(), logger, *profile))

			service := store.Service{App: conf.App, Stack: conf.Stack, Stage: conf.Stage}
			items, err := ssm.List(service)
			check(logger, err, fmt.Sprintf("unable to list for service '%s'", service.Prefix()), 1)

			for _, item := range items {
				logger.Infof(item.String())
			}
		},
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Set parameter for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to set")
			value := cmd.Flags().String("value", "", "Value of parameter to set")
			cmd.MarkFlagRequired("name")
			cmd.MarkFlagRequired("value")
			cmd.ParseFlags(args)

			argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
			conf, err := config.Read(argConf, config.DefaultFiles()...)
			check(logger, err, "Unable to read config", InvalidArgs)

			ssm := store.NewSSM(logger, ssmClient(context.TODO(), logger, *profile))
			service := store.Service{App: conf.App, Stack: conf.Stack, Stage: conf.Stage}

			isSecret := askYesNo("Is this parameter a secret?")

			err = ssm.Set(service, *name, *value, isSecret)
			check(logger, err, fmt.Sprintf("unable to set '%s=%s' for service '%s'", *name, *value, service.Prefix()), 1)
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete parameter for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to set")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
			conf, err := config.Read(argConf, config.DefaultFiles()...)
			check(logger, err, "Unable to read config", InvalidArgs)

			ok := askYesNo(fmt.Sprintf("Are you sure you want to delete '%s'?", *name))
			if !ok {
				logger.Infof("Config item '%s' has NOT been deleted.", name)
				return
			}

			ssm := store.NewSSM(logger, ssmClient(context.TODO(), logger, *profile))
			service := store.Service{App: conf.App, Stack: conf.Stack, Stage: conf.Stage}

			err = ssm.Delete(service, *name)
			check(logger, err, fmt.Sprintf("unable to delete '%s' for service '%s'", *name, service.Prefix()), 1)
		},
	}

	setConfig := &cobra.Command{
		Use:   "set-local-config",
		Short: "Set local config (app, stack, stage) for a service to automatically set these in the future",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(args)

			argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
			conf, err := config.Read(argConf) // note, don't check existing files

			if err != nil {
				app := ask("App: ")
				stack := ask("Stack: ")
				stage := ask("Stage: ")

				conf = config.Config{App: app, Stack: stack, Stage: stage}
			}

			config.Write(conf)
		},
	}

	rootCmd.AddCommand(getCmd, listCmd, setCmd, deleteCmd, setConfig)
	rootCmd.Execute()

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
	cfg, err := awsConfig.LoadDefaultConfig(ctx, awsConfig.WithSharedConfigProfile(profile), awsConfig.WithRegion("eu-west-1"))
	check(logger, err, "unable to load default config", 1)
	return ssm.NewFromConfig(cfg)
}

func readBoolFlag(args []string, name string, usage string) bool {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.Usage = func() {} // silence errors
	got := fs.Bool(name, false, usage)
	fs.Parse(args)
	return *got
}

func check(logger log.Logger, err error, msg string, exitCode int) {
	if err != nil {
		logger.Infof("%s; %v", msg, err)
		os.Exit(exitCode)
	}
}
