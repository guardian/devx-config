package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/spf13/cobra"
)

func main() {
	// app stack stage

	// attempt to read tags from the instance-metadata file.
	// also group everything into a single repository!

	// @guardian/devx
	// /logs
	// /config
	// /tags

	// Would be great if these could all include easy validation of config and also a template GHA YAML (or generate this somehow).
	// @guardian/actions-static-site
	// @guardian/actions-lambda
	// @guardian/actions-ec2

	// devx-config get-all --no-echo --app foo --stack deploy --stage INFRA --profile deployTools
	// get
	// set
	// delete

	rootCmd := &cobra.Command{
		Use:   "devx-config",
		Short: "Manages app configuration.",
	}

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Gets specific config for the stack,stage,app combo.",
		Run: func(cmd *cobra.Command, args []string) {
			println("TODO...")
		},
	}

	getAllCmd := &cobra.Command{
		Use:   "get-all",
		Short: "Gets all config for the stack,stage,app combo.",
		Run: func(cmd *cobra.Command, args []string) {
			println("TODO...")
		},
	}

	setCmd := &cobra.Command{
		Use:   "set",
		Short: "Sets specific config for the stack,stage,app combo.",
		Run: func(cmd *cobra.Command, args []string) {
			println("TODO...")
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Deletes specific config for the stack,stage,app combo.",
		Run: func(cmd *cobra.Command, args []string) {
			println("TODO...")
		},
	}

	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(getAllCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(deleteCmd)

	rootCmd.Execute()

	prefix := flag.String("prefix", "", "Parameter store prefix")
	flag.Parse()
	if *prefix == "" {
		log.Fatal("Error: required flag 'prefix' missing.")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile("deployTools"), config.WithRegion("eu-west-1"))
	check(err, "unable to load default config")

	client := ssm.NewFromConfig(cfg)

	input := &ssm.GetParametersByPathInput{
		Path:           prefix,
		Recursive:      true,
		WithDecryption: true,
	}

	output, err := client.GetParametersByPath(ctx, input)
	check(err, "unable to read from parameter store")

	fmt.Print(asKV(output.Parameters, *prefix, false))
}

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
