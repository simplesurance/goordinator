# Goordinator

## Introduction

Goordinator listens for GitHub webhook events, runs their JSON payloads through
a a JQ filter query and triggers actions if the filter matches.

The only supported action is currently posting HTTP-Requests.
All actions are executed in parallel and retried if they fail until a retry
timeout expired (default: 2h).

## Installation as systemd Service

1. Download a release archive from: <https://github.com/simplesurance/goordinator/releases>.
2. Extract the archive to a temporary directory.
3. Install `goordinator` to `/usr/local/bin`:

   ```sh
   install -o root -g root -m 775 goordinator /usr/local/bin/goordinator
   ```

4. Copy the configuration files to `/etc`:

   ```sh
   cp -r dist/etc/* /etc/
   chmod 660 /etc/conf.d/goordinator /etc/goordinator/rules.toml
   ```

5. Adapt the configuration in `/etc/conf.d/goordinator` and define your rules in
   `/etc/goordinator/rules.toml`.
6. Enable the systemd service and start it:

   ```sh
   systemctl daemon-reload
   systemctl enable goordinator
   systemctl start goordinator`
   ```

## Configuration

Goordinator is configured via a `rules.toml` file and commandline parameters or
environment variables .

### Rules Configuration File

A documented example configuration file can be found in the repository:
[dist/etc/goordinator/rules.toml](dist/etc/goordinator/rules.toml).

### Commandline Parameters / Environment Variables

See the output of `goordinator --help`.

### Template Strings

The configuration options of actions can contain template strings. The template
strings are replaced with concrete values from the event that is processed.

The supported template strings are documented in the
[example rules.toml file](dist/etc/goordinator/rules.toml).

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.
