package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"os"
	"time"

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

	secretRecoveryWindow := rootCmd.PersistentFlags().Int64("secret-recovery-window", 30, "Recovery window when deleting secrets from Secret Manager. Set to 0 to disable recovery (use with caution!)")
	timeout := rootCmd.PersistentFlags().Int64("timeout", 5, "Number of seconds to wait for AWS operations")

	argConf := config.Config{App: *app, Stack: *stack, Stage: *stage}
	conf, configErr := config.Read(argConf, config.DefaultFiles()...)

	service := store.Service{App: conf.App, Stack: conf.Stack, Stage: conf.Stage}

	ssmStore, secretStore := createStores(context.Background(), logger, *profile, *secretRecoveryWindow, time.Duration(*timeout)*time.Second)

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get parameter for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to retrieve")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			item, err := ssmStore.Get(service, *name)
			check(logger, err, fmt.Sprintf("unable to get %s for service '%s'", *name, service.Prefix()), 1)

			logger.Infof(item.String())
		},
	}

	getSecretCmd := &cobra.Command{
		Use:   "get-secretmgr",
		Short: "Get secret from secrets manager",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of the secret to retrieve")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			item, err := secretStore.Get(service, *name)
			check(logger, err, fmt.Sprintf("unable to retrieve secrets %s for %s", *name, service.Prefix()), InternalError)

			logger.Infof(item.String())
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all parameters for a service",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			items, err := ssmStore.List(service)
			check(logger, err, fmt.Sprintf("unable to list for service '%s'", service.Prefix()), InternalError)

			for _, item := range items {
				logger.Infof(item.String())
			}
		},
	}

	listSecretCmd := &cobra.Command{
		Use:   "list-secretmgr",
		Short: "List all secrets for the service from secrets manager",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			items, err := secretStore.List(service)
			check(logger, err, fmt.Sprintf("unable to list secrets for service '%s'", service.Prefix()), InternalError)

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

			check(logger, configErr, "Unable to read config", InvalidArgs)

			isSecret := askYesNo("Is this parameter a secret?")

			configErr = ssmStore.Set(service, *name, *value, isSecret)
			check(logger, configErr, fmt.Sprintf("unable to set '%s=%s' for service '%s'", *name, *value, service.Prefix()), InternalError)
		},
	}

	setSecretCmd := &cobra.Command{
		Use:   "set-secretmgr",
		Short: "Set secret in secrets manager for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to set")
			value := cmd.Flags().String("value", "", "Value of parameter to set")
			cmd.MarkFlagRequired("name")
			cmd.MarkFlagRequired("value")
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			configErr := secretStore.Set(service, *name, *value, true)
			check(logger, configErr, fmt.Sprintf("unable to set secret '%s=%s' for service %s", *name, *value, service.Prefix()), InternalError)
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete parameter for a service",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name of parameter to delete")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			ok := askYesNo(fmt.Sprintf("Are you sure you want to delete '%s'?", *name))
			if !ok {
				logger.Infof("Config item '%s' has NOT been deleted.", name)
				return
			}

			configErr = ssmStore.Delete(service, *name)
			check(logger, configErr, fmt.Sprintf("unable to delete '%s' for service '%s'", *name, service.Prefix()), 1)
		},
	}

	deleteSecretCmd := &cobra.Command{
		Use:   "delete-secretmgr",
		Short: "Delete a secret for the service from Secrets Manager",
		Run: func(cmd *cobra.Command, args []string) {
			name := cmd.Flags().String("name", "", "Name or ARN of secret to delete")
			cmd.MarkFlagRequired("name")
			cmd.ParseFlags(args)

			check(logger, configErr, "Unable to read config", InvalidArgs)

			ok := askYesNo(fmt.Sprintf("Are you sure you want to delete '%s'?", *name))
			if !ok {
				logger.Infof("Secret '%s' has NOT been deleted.", name)
				return
			}

			configErr = secretStore.Delete(service, *name)
			check(logger, configErr, fmt.Sprintf("unable to delete secret '%s' for service '%s'", *name, service.Prefix()), 1)
		},
	}
	setConfig := &cobra.Command{
		Use:   "set-local-config",
		Short: "Set local config (app, stack, stage) for a service to automatically set these in the future",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.ParseFlags(args)

			// note, don't check existing files

			if configErr != nil {
				app := ask("App: ")
				stack := ask("Stack: ")
				stage := ask("Stage: ")

				conf = config.Config{App: app, Stack: stack, Stage: stage}
			}

			config.Write(conf)
		},
	}

	rootCmd.AddCommand(getCmd, listCmd, setCmd, deleteCmd, setConfig, getSecretCmd, listSecretCmd, setSecretCmd, deleteSecretCmd)
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

func createStores(ctx context.Context, logger log.Logger, profile string, secretRecoveryWindow int64, defaultTimeout time.Duration) (store.Store, store.Store) {
	cfg, err := awsConfig.LoadDefaultConfig(ctx, awsConfig.WithSharedConfigProfile(profile), awsConfig.WithRegion("eu-west-1"))
	check(logger, err, "unable to load default config", 1)
	ssmClient := ssm.NewFromConfig(cfg)
	ssmStore := store.NewSSM(logger, ssmClient)
	secretsMgrClient := secretsmanager.NewFromConfig(cfg)
	secretsMgrStore, err := store.NewSecretsManager(secretsMgrClient, secretRecoveryWindow, logger, defaultTimeout)

	check(logger, err, "unable to initialise Secrets Manager", 1)
	return ssmStore, secretsMgrStore
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
