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
   chmod 660 /etc/goordinator/config.toml
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

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.

---------
Homepage: <https://github.com/simplesurance/goordinator>
