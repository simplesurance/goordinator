# Goordinator


## Introduction

Goordinator listens for GitHub webhook events, runs the JSON payloads through a
a JQ filter query and triggers actions if the filter matches.

As actions currently only sending HTTP-Requests is supported.
All actions are executed in parallel and retried if they fail until a retry
timeout expired (default: 2h).

## Configuration

Goordinator is configured via a `rules.toml` file.

The filter_query can template strings.
The template strings are replaced when the event is received with information
from the event.

Example:

```toml
[[rule]]
  name = "trigger-ci-jobs"
  event_source = "github"
  filter_query = '''
.repository.name == "testrepo" and (
  .action == "synchronize" and ([.pull_request.labels[].name] | map(select(. == "ci")) | length) > 0
) or (
  .action == "labeled" and .label.name == "ci"
)
'''

# All actions are executed in parallel and retried on errors
  [[rule.action]]
    action = "httprequest"
    url = "https://myjenkins/view/CI/job/checks/view/change-requests/job/PR-{{ .Event.PullRequestNr }}/build"
    user = "fho"
    password = "<token>"

  [[rule.action]]
    action = "httprequest"
    url = "https://circleci.com/api/v2/project/gh/goordinator/checks/pipeline"
    user = "<token">
    headers = { "content-type" = "application/json" }
    data = '''
    {
      "branch": "pull/14380/head"
    }
    '''
```

### filter_query: Supported Template Strings

wip

## FAQ

#### Github Hook Events did not arrive because the Application was unreachable

Trigger a redelivery of the lost events in the GitHub settings page of the
repository.
