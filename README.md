# devx-config

Configuration and secret management for Guardian applications. `devx-config` is
both a command-line tool for managing application configuration, and a runtime
tool for passing configuration (as environment variables) to your application.

To install:

    $ go install github.com/guardian/devx-config
    $ devx-config --help

If you haven't installed Go already, run `brew install go`.

## Managing configuration

`devx-config` supports CRUD-like operations for configuration and secret
management. Behind the scenes, configuration is written to AWS Parameter Store,
though that should be considered an implementation detail and subject to change
over time. The key thing is that it provides an audit trail of changes over time
via AWS Cloudtrail.

## App requirements

To use `devx-config`, your EC2 application needs the following:

- to read its configuration and secrets via environment variables
- to be running on an instance with SSM read permissions for
  `/:stage/:stack/:app/*`
