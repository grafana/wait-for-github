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
   --commit-info-file value  Path to a file which the commit info will be written. The file will be overwritten if it already exists.
   --help, -h                show help
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
   --check value, -c value [ --check value, -c value ]  Check the status of a specific CI check. By default, the status of all checks is checked. [$GITHUB_CI_CHECKS]
   --help, -h  show help (default: false)
```

This command will wait for CI checks to finish for a ref. If they finish
successfully it will exit `0` and otherwise it will exit `1`.

## Action

This repository also contains a GitHub action definition. You can add this as a
step to your workflow to sync running steps after CI has finished or a PR has
been merged.


### Action parameters

#### `ref`

**Required**. Git ref to check.

#### `wait-for`

**Required**. What to wait for. Valid values are `"ci"` or `"pr"`.

- `"ci"`: Waits for the CI to finish. 
- `"pr"`: Waits for the PR to be merged.


#### `checks-to-wait-for`

The comma-separated names of the checks to wait for. Use this when your checks
might take some time to be reported to GitHub and they aren't marked as required
checks on the repository. Only used when `wait-for` is set to `"ci"`. Optional.
If not set, all checks will be waited for, but then the tool may miss checks
which are not added immediately.

#### `owner`

GitHub repo owner. Optional. Default is the current repository's owner,
`${{ github.repository_owner }}`.

#### `interval`

Recheck interval (i.e. poll this often) in golang duration format. Optional.
Default is `30s`.

#### `repo`

GitHub repo name. Optional. Default is the current repository, 
`${{ github.event.repository.name }}`.

#### `token`

The GitHub token to use. Optional. Default is the github token provided by the
action.

#### `timeout`

Timeout in golang duration format. Optional. Default is `5m`.

### Example usage

Since the project is not yet versioned, we recommend that you use a specific SHA
when using the action, rather than taking the latest which might break
compatibility.

```yaml
name: Foo the bar if the Atlantis plan succeeds
on:
  pull_request:
    branches:
      - main

permissions:
   checks: read
   contents: read
   pull-requests: write
   statuses: read

jobs:
  wait-for-checks:
    runs-on: ubuntu-latest
    steps:
      - name: Wait for Atlantis to plan
        uses: grafana/wait-for-github@SHA
        id: wait-for-checks
        with:
          wait-for: ci
          checks-to-wait-for: atlantis/plan
          ref: ${{ github.event.pull_request.head.sha }}
      - name: Comment on check failure
        if: failure()
        run: |
          gh pr review --comment -b "Could not foo this PR because the Atlantis plan failed." "$PR_URL"
        env:
          PR_URL: ${{github.event.pull_request.html_url}}
          GITHUB_TOKEN: ${{secrets.GITHUB_TOKEN}}
      - name: Do something if the plan succeeded
        run: |
          gh pr review --comment -b "Yay I'm so happy that the plan succeeded!" "$PR_URL"
        env:
          PR_URL: ${{github.event.pull_request.html_url}}
          GITHUB_TOKEN: ${{secrets.GITHUB_TOKEN}}
```

## Contributing

Contributions via issues and GitHub PRs are very welcome. We'll try to be
responsive!

## Versioning

As of now, the project is not versioned. We recommend that you use the `latest`
tag. We'll _try_ to keep compatibility but there are no guarantees until we do
start using semantic versioning.
