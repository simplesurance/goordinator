# Goordinator

## Introduction

Goordinator listens for GitHub webhook events, runs their JSON payloads through
a a JQ filter query and triggers actions if the filter matches.

As actions currently only posting HTTP-Requestes are supported.
All actions are executed in parallel and retried if they fail until a retry
timeout expired (default: 2h).

## Installation

Run:

```sh
make
```

and copy the `goordinator` binary to a place of your choice.


## Configuration

Goordinator is configured via a `rules.toml` file and commandline parameters.

### Rules Configuration File

A documented example configuration file can be found [rules.example.toml](here).

### Commandline Parameters

See the output of `goordinator --help`.

### Template Strings

The configuration options of actions (`httprequest`) can
contain template strings. The template strings are replaced with concrete values
from the event that is processed.

The supported template strings are documented in [rules.example.toml].

### Logging

Log messages are printed to STDOUT in JSON structured log format by default.
The format can be changed to a slightly more readable format via the
`--log-format` commandline parameter.

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.
