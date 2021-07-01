# Goordinator

## Introduction

Goordinator is an event-processor for GitHub events.
It provides 2 functionalities.

### Configurable Event-Trigger Loop

Goordinator listens for GitHub webhook events, runs their JSON payloads
through a JQ filter query and triggers actions if the filter matches.
The supported actions are:
- posting a http-request
- updating a GitHub branch with it's base branch.

All actions are executed in parallel and retried if they fail until a retry
timeout expired (default: 2h).

### Serialized GitHub Branch Autoupdater

Autoupdater keeps Pull-Requests (PR) updated with their base branch.
Pull-Requests are added to a per base branch queue and the first pull-request in
the queue is kept up to date with its base branch.
If merging the base branch into the PR branch fails, its check status becomes
negative or the PR became stale, updates for it are suspended.
Updates for it are resumed when the base-branch or the PR branch changed or the
check status of the PR became positive.

Autoupdater is used together with [Github's auto-merge
feature](https://docs.github.com/en/github/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request)
or a comparable service to provide a serialized merge-queue.
The autoupdater serializes updates per base branch, to avoid a race between
pull-requests to get updates the fastest and have a successful CI check first.

Without an external auto-merge service the autoupdater is useless.

#### Required GitHub Setup

Configure your GitHub Repository to:

- Enable auto-merge
- Require >=1 status checks to pass before merging
- Require branches to be up to date before merging

## Installation as systemd Service

1. Download a release archive from: <https://github.com/simplesurance/goordinator/releases>.
2. Extract the archive to a temporary directory.
3. Install `goordinator` to `/usr/local/bin`:

   ```sh
   install -o root -g root -m 775 goordinator /usr/local/bin/goordinator
   ```
4. Create a goordinator group and user:

   ```sh
   useradd -r goordinator
   ```

4. Copy the configuration files to `/etc`:

   ```sh
   cp -r dist/etc/* /etc/
   chmod 660 /etc/goordinator/config.toml
   chown root:goordinator /etc/goordinator/config.toml
   ```

5. Adapt the configuration and define your rules in
   `/etc/goordinator/config.toml`.
6. Enable the systemd service and start it:

   ```sh
   systemctl daemon-reload
   systemctl enable goordinator
   systemctl start goordinator
   ```

## Configuration

Goordinator is configured via a `config.toml` file. \
The path to the configuration file can be defined via `--cfg-file` commandline
parameter. \
A documented example configuration file can be found in the repository:
[dist/etc/goordinator/config.toml](dist/etc/goordinator/config.toml).

### Template Strings

The configuration options of actions can contain template strings. The template
strings are replaced with concrete values from the event that is processed.

The supported template strings are documented in the
[example config.toml file](dist/etc/goordinator/config.toml).

## Project Status

The project is an early stage. Breaking changes happen anytime. \
Test coverage is insufficient, therefore bugs *main* branch are likely.

The release binaries are in a more stable state. They are roughly tested and
used internally.

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.

---------
Homepage: <https://github.com/simplesurance/goordinator>
