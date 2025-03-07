# Goordinator

## Introduction

Goordinator is a configurable event-processor for GitHub events.
It listens for GitHub webhook events, runs their JSON payloads
through a query filter (jq) and runs actions when a filter matches. The
supported actions are:

- sending a HTTP-request,
- updating a GitHub branch with its base branch,
- removing a GitHub pull request label.

All actions are executed in parallel and retried on failures.

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

5. Copy the configuration files to `/etc`:

   ```sh
   cp -r dist/etc/* /etc/
   chmod 660 /etc/goordinator/config.toml
   chown root:goordinator /etc/goordinator/config.toml
   ```

6. Adapt the configuration and define your rules in
   `/etc/goordinator/config.toml`.
7. Enable the systemd service and start it:

   ```sh
   systemctl daemon-reload
   systemctl enable goordinator
   systemctl start goordinator
   ```

## Configuration

Goordinator is configured via a `config.toml` file. \
The path to the configuration file can be defined via the `--cfg-file` command
line parameter. \
A documented example configuration file can be found in the repository:
[dist/etc/goordinator/config.toml](dist/etc/goordinator/config.toml).

### Template Strings

The configuration options of actions can contain template strings. The template
strings are replaced with concrete values from the event that is processed.

The supported template strings are documented in the
[example config.toml file](dist/etc/goordinator/config.toml).


### Example Rules

#### Run Jenkins Job by Adding a PR Label

```toml
[[rule]]
  name = "run-ci-job-on-label"
  event_source = "github"
  filter_query = '''
.repository.name == "repo.example" and
  .action == "labeled" and .label.name == "ci"
'''

  [[rule.action]]
    action = "httprequest"
    url = "https://jenkins.test/job/ci/buildWithParameters?version={{ .Event.CommitID }}"
    user = "goordinator"
    password = "1234"

  [[rule.action]]
    action = "removelabel"
    label = "ci"
```

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.

---------
Homepage: <https://github.com/simplesurance/goordinator>
