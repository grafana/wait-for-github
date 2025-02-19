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
   --log-level value, -l value                    Set the log level. Valid levels are: error, warn, info, and debug. (default: "info")
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

### Required Permissions

The GitHub token or app needs the following permissions:

- `actions:read` - Read names of workflows which ran on a ref
- `checks:read` - Read check run status and conclusions for CI checks
- `contents:read` - Access commit data through GitHub's GraphQL API when
  checking CI status
- `metadata:read` - Basic access to repository information and API endpoints
- `pull-requests:read` - Check if PRs have been merged or closed
- `statuses:read` - Read commit status checks when verifying CI completion

If using a GitHub App, configure these permissions when setting up the app. If
using a Personal Access Token (PAT), make sure to select these scopes when
creating the token.

### Commands

#### `pr`

```
NAME:
   wait-for-github pr - Wait for a PR to be merged

USAGE:
   wait-for-github pr [command options] <https://github.com/OWNER/REPO/pulls/PR|owner> [<repo> <pr>]

OPTIONS:
   --commit-info-file value  Path to a file which the commit info will be written. The file will be overwritten if it already exists.
   --exclude value, -x value [ --exclude value, -x value ]  Exclude the status of a specific CI check from failing the wait. By default, a failed status check will exit the pr wait command. [$GITHUB_CI_EXCLUDE]
   --help, -h                show help
```

This command will wait for the given PR (URL or owner/repo/number) to be merged
or closed. If merged, it will exit with code `0` (success) and if closed without
being merged it will exit with code `1` (failure).

#### `ci`

```
NAME:
   wait-for-github ci - Wait for CI to be finished

USAGE:
   wait-for-github ci [command options] <https://github.com/OWNER/REPO/commit|pull/HASH|PRNumber|owner> [<repo> <ref>]

OPTIONS:
   --check value, -c value [ --check value, -c value ]  Check the status of a specific CI check. By default, the status of all required checks is checked. [$GITHUB_CI_CHECKS]
   --exclude value, -x value [ --exclude value, -x value ]  Exclude the status of a specific CI check. Argument ignored if checks are specified individually. By default, the status of all checks is checked. [$GITHUB_CI_EXCLUDE]
   --help, -h  show help (default: false)
```

This command will wait for CI checks to finish for a ref or PR URL. If they finish
successfully it will exit `0` and otherwise it will exit `1`.

To wait for a specific check to finish, use the `--check` flag. To exclude
specific checks from failing the status, use the `--exclude` flag. See below for
details of the `ci list` subcommand, which can help determine valid values for
this flag. This flag can be given multiple times to wait for multiple checks. To
wait for the result of a GitHub Actions workflow, pass the base name of the
workflow as the check name. For example, `.github/workflows/lint.yml` would be
`lint`. To wait for [commit statuses][statuses], use the name of the status as
shown in the GitHub web UI.

##### `ci list`

The `ci list` subcommand can be used to list all CI checks and their current status:

```console
$ wait-for-github ci list https://github.com/grafana/wait-for-github/pull/123
╒═══════════════════════╤═══════════╤═════════╕
│         NAME          │   TYPE    │ STATUS  │
╞═══════════════════════╪═══════════╪═════════╡
│ **test**              │ Status    │ Passed  │
│ linters / **black**   │ Action    │ Failed  │
│ **deploy**            │ Check Run │ Pending │
╘═══════════════════════╧═══════════╧═════════╛
```

This is useful to see what checks are available to pass to the `--check` or
`--exclude` flag. Denoted by `**` in the output above, part of the check name
will be in bold. These names can be used as values for the `ci --check` or
`ci --exclude` flags.

[statuses]: https://docs.github.com/en/rest/commits/statuses

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

#### `app-id`, `app-private-key`, `app-installation-id`

The GitHub Application ID, App Private Key and App Installation ID. Optional.
Use in a case of the authentication with a GitHub App (as an alternative to
GitHub Token auth).

#### `checks-to-wait-for`

The comma-separated names of the checks to wait for. Use this when your checks
might take some time to be reported to GitHub and they aren't marked as required
checks on the repository. Only used when `wait-for` is set to `"ci"`. Optional.
If not set, all checks will be waited for, but then the tool may miss checks
which are not added immediately.

#### `interval`

Recheck interval (i.e. poll this often) in golang duration format. Optional.
Default is `30s`.

#### `owner`

GitHub repo owner. Optional. Default is the current repository's owner,
`${{ github.repository_owner }}`.

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

`wait-for-github` follows [Semantic Versioning]. Breaking changes will result in
a major version bump, new features will result in a minor version bump, and bug
fixes will result in a patch version bump.

[Semantic Versioning]: https://semver.org/
