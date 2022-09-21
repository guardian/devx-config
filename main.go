package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/spf13/cobra"
)

type Service struct {
	Stack, Stage, App string
}

func (s Service) Prefix() string {
	return fmt.Sprintf("/%s/%s/%s", s.Stage, s.Stack, s.App)
}

func main() {
	// attempt to read tags from the instance-metadata file.

	// also group everything into a single repository:
	// @guardian/devx
	//   /logs
	//   /config
	//   /tags

	// Would be great if these could all include easy validation of config and also a template GHA YAML (or generate this somehow).
	// @guardian/actions-static-site
	// @guardian/actions-lambda
	// @guardian/actions-ec2

	ctx := context.Background()

	rootCmd := &cobra.Command{
		Use:   "devx-config",
		Short: "Manages app configuration.",
	}

	stack := rootCmd.PersistentFlags().String("stack", "", "Guardian stack of the app.")
	stage := rootCmd.PersistentFlags().String("stage", "", "Guardian stage of the app (e.g. CODE, PROD, INFRA.")
	app := rootCmd.PersistentFlags().String("app", "", "App name.")
	profile := rootCmd.PersistentFlags().StringP("profile", "p", "", "AWS profile for credentials. (Only required when running locally.)")

	rootCmd.MarkFlagRequired("stack")
	rootCmd.MarkFlagRequired("stage")
	rootCmd.MarkFlagRequired("app")

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Gets specific config for the stack,stage,app combo.",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			service := Service{App: *app, Stack: *stack, Stage: *stage}
			out := get(ctx, ssmClient(ctx, *profile), service, "TODO")
			println(out)
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "Lists all config for the stack,stage,app combo.",
		Run: func(cmd *cobra.Command, args []string) {
			service := Service{App: *app, Stack: *stack, Stage: *stage}
			out := list(ctx, ssmClient(ctx, *profile), service)
			println(out)
		},
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Sets specific config for the stack,stage,app combo.",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			service := Service{App: *app, Stack: *stack, Stage: *stage}
			set(ctx, ssmClient(ctx, *profile), service, "NAME", "VALUE")
		},
	}

	var parameter string
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes specific config for the stack,stage,app combo.",
		Args:  cobra.MinimumNArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			service := Service{App: *app, Stack: *stack, Stage: *stage}

			fmt.Printf("Type %s to confirm parameter deletion: ", parameter)
			var confirmation string
			fmt.Scanln(&confirmation)

			if confirmation != parameter {
				fmt.Println("Error: parameter name does not match. Exiting...")
				os.Exit(1)
			}

			delete(ctx, ssmClient(ctx, *profile), service)
		},
	}
	deleteCmd.Flags().StringVar(&parameter, "parameter", "", "The parameter you want to delete.")

	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(deleteCmd)

	rootCmd.Execute()
}

func ssmClient(ctx context.Context, profile string) *ssm.Client {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(profile), config.WithRegion("eu-west-1"))
	check(err, "unable to load default config")
	return ssm.NewFromConfig(cfg)

}

func get(ctx context.Context, client *ssm.Client, service Service, name string) string {
	output, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           &name,
		WithDecryption: true,
	})
	check(err, fmt.Sprintf("unable to get parameter '%s'", name))

	return asKV([]types.Parameter{*output.Parameter}, service.Prefix(), false)
}

func list(ctx context.Context, client *ssm.Client, service Service) string {
	pages := ssm.NewGetParametersByPathPaginator(client, &ssm.GetParametersByPathInput{
		Path:           aws.String(service.Prefix()),
		WithDecryption: true,
	})

	var params []types.Parameter
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		check(err, "unable to list parameters")

		params = append(params, page.Parameters...)
	}

	return asKV(params, service.Prefix(), true)
}

func set(ctx context.Context, client *ssm.Client, service Service, name string, value string) {}

func delete(ctx context.Context, client *ssm.Client, service Service) {}

func asKV(params []types.Parameter, prefix string, noEcho bool) string {
	builder := strings.Builder{}

	for _, param := range params {
		name := *param.Name
		value := *param.Value

		if param.Type == types.ParameterTypeSecureString && noEcho {
			value = "******"
		}

		builder.WriteString(fmt.Sprintf("%s=%s\n", clean(name, prefix), value))
	}

	return builder.String()
}

func clean(s, prefix string) string {
	r := strings.NewReplacer(prefix+"/", "", ".", "_", "/", "_")
	return r.Replace(s)
}

func check(err error, msg string) {
	if err != nil {
		log.Fatalf("%s; %v", msg, err)
	}
}
