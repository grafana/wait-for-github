# Contributing

Thank you for your interest in contributing to Grafana `wait-for-github`
project! We welcome all people who want to contribute in a healthy and
constructive manner within our community. To help us create a safe and positive
community experience for all, we require all participants to adhere to the [Code
of Conduct][code-of-conduct].

This document is a guide to help you through the process of contributing to `wait-for-github`.

## Become a contributor

You can contribute to Grafana `wait-for-github` in several ways. Here are some
examples:

- Contribute to the codebase itself.
- Report bugs and enhancements.
- Help with maintaining the project, for example by responding to issues and
  creating releases.

For more ways to contribute, check out the [Open Source
Guides][open-source-guides].

### Report bugs

Report a bug by submitting a [bug
report][bug-report].
Make sure that you provide as much information as possible on how to reproduce
the bug.

Before submitting a new issue, try to make sure someone hasn't already reported
the problem. Look through the [existing
issues][existing-issues] for similar issues.

#### Security issues

If you believe you've found a security vulnerability, please read our [security
policy][security-policy] for more details.

## Creating releases

We use [`release-please`][release-please] to create releases. This will maintain
a draft pull request with the changes needed to bump the version and update the
changelog, updated with the changes since the last release.

Releasing should be as simple as merging that pull request. Check that a GitHub
release, a tag, and the Docker releases were created.

### Use conventional commit messages

As we're using [`release-please`][release-please] to create releases, we need to use
conventional commit messages. This means that each commit message should follow
the format:

```
<type>[optional scope]: <description>
```

See the [conventional commits specification][conventional-commits] for more details.

A CI check is in place which will enforce this format.

[bug-report]: https://github.com/grafana/wait-for-github/issues/new?labels=bug&template=1-bug_report.md
[code-of-conduct]: CODE_OF_CONDUCT.md
[conventional-commits]: https://www.conventionalcommits.org
[existing-issues]: https://github.com/grafana/wait-for-github/issues
[open-source-guides]: https://opensource.guide/how-to-contribute/
[release-please]: https://github.com/googleapis/release-please
[security-policy]: https://github.com/grafana/wait-for-github/security/policy
