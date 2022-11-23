# devx-config (NOT YET READY FOR USE)

Configuration and secret management for Guardian applications. `devx-config` is
both a command-line tool for managing application configuration, and a runtime
tool for passing configuration (as environment variables) to your application.

Behind the scenes AWS Parameter Store is used for storage.

To install:

    $ go install github.com/guardian/devx-config
    $ devx-config -h

If you haven't installed Go already, run `brew install go`.

## Managing configuration (locally)

CRUD-like subcommands (`list`, `get`, `set`, `delete`) are available for
managing configuration. E.g.

  $ devx-config set --profile=[profile] --app=[app] --stack=[stack] --stage=[STAGE] --name=[name] --value=[value]

To save time, you can add a local config file in your repo (or a subdirectory
within it) to store the boilerplate args:

```
// .devx-config
{
  "App": "my-app",
  "Stack": "my-stack",
  "Stage": "PROD",
}
```

Run the `set--local-onfig` command to create this file. E.g.

  $ devx-config set-local-config --app[app] --stack=[stack] --stage=[STAGE]

Future commands (when run from the same directory) will take the app/stack/stage
args from there.

## App requirements

To use `devx-config`, your EC2 application needs the following:

- to read its configuration and secrets via environment variables
- to be running on an instance with SSM read permissions for
  `/:stage/:stack/:app/*`

