# `wait-for-github`

This is a small program that can wait for things to happen on GitHub.

## Usage

```
NAME:
   wait-for-github - Wait for things to happen on GitHub

USAGE:
   wait-for-github [global options] command [command options] [arguments...]

COMMANDS:
   ci       Wait for CI to be finished
   pr       Wait for a PR to be merged
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --log-level value, -l value                    Set the log level. Valid levels are: panic, fatal, error, warning, info, debug, trace. (default: "info")
   --github-app-private-key-path value, -p value  Path to the GitHub App private key
   --github-app-private-key value                 Contents of the GitHub App private key [$GITHUB_APP_PRIVATE_KEY]
   --github-app-id value                          GitHub App ID (default: 0) [$GITHUB_APP_ID]
   --github-app-installation-id value             GitHub App installation ID (default: 0) [$GITHUB_APP_INSTALLATION_ID]
   --github-token value                           GitHub token. If not provided, the app will try to use the GitHub App authentication mechanism. [$GITHUB_TOKEN]
   --recheck-interval value                       Interval after which to recheck GitHub. (default: 30s) [$RECHECK_INTERVAL]
   --timeout value                                Timeout after which to stop checking GitHub. (default: 168h0m0s) [$TIMEOUT]
   --help, -h                                     show help (default: false)
```

Authentication is via either a GitHub personal access token or an app private
key, ID and installtion ID.

### Commands

#### `pr`

```
NAME:
   wait-for-github pr - Wait for a PR to be merged

USAGE:
   wait-for-github pr [command options] <https://github.com/OWNER/REPO/pulls/PR|owner> [<repo> <pr>]

OPTIONS:
   --help, -h  show help (default: false)
```

This command will wait for the given PR (URL or owner/repo/number) to be merged
or closed. If merged, it will exit with code `0` (success) and if cloed without
being merged it will exit with code `1` (failure).

#### `ci`

```
NAME:
   wait-for-github ci - Wait for CI to be finished

USAGE:
   wait-for-github ci [command options] <https://github.com/OWNER/REPO/commit/HASH|owner> [<repo> <ref>]

OPTIONS:
   --help, -h  show help (default: false)
```
This command will wait for CI checks to finish for a ref. If they finish successfully it will exit `0` and otherwise it will exit `1`.

## Contributing

Contributions via issues and GitHub PRs are very welcome. We'll try to be
responsive!

## Versioning

As of now, the project is not versioned. We recommend that you use the `latest`
tag. We'll _try_ to keep compatibility but there are no guarantees until we do
start using semantic versioning.
