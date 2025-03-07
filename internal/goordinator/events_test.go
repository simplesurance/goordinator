package goordinator

import (
	"io"
	"net/http"
	"strings"
)

const pullRequestSynchronizeEventPayload = `
{
  "action": "synchronize",
  "number": 1,
  "pull_request": {
    "url": "https://api.github.com/repos/fho/testrepo/pulls/1",
    "id": 639643511,
    "node_id": "MDExOlB1bGxSZXF1ZXN0NjM5NjQzNTEx",
    "html_url": "https://github.com/fho/testrepo/pull/1",
    "diff_url": "https://github.com/fho/testrepo/pull/1.diff",
    "patch_url": "https://github.com/fho/testrepo/pull/1.patch",
    "issue_url": "https://api.github.com/repos/fho/testrepo/issues/1",
    "number": 1,
    "state": "open",
    "locked": false,
    "title": "add comment to readme",
    "user": {
      "login": "fho",
      "id": 514535,
      "node_id": "MDQ6VXNlcjUxNDUzNQ==",
      "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/fho",
      "html_url": "https://github.com/fho",
      "followers_url": "https://api.github.com/users/fho/followers",
      "following_url": "https://api.github.com/users/fho/following{/other_user}",
      "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
      "organizations_url": "https://api.github.com/users/fho/orgs",
      "repos_url": "https://api.github.com/users/fho/repos",
      "events_url": "https://api.github.com/users/fho/events{/privacy}",
      "received_events_url": "https://api.github.com/users/fho/received_events",
      "type": "User",
      "site_admin": false
    },
    "body": "",
    "created_at": "2021-05-11T07:39:11Z",
    "updated_at": "2021-05-11T07:40:47Z",
    "closed_at": null,
    "merged_at": null,
    "merge_commit_sha": null,
    "assignee": null,
    "assignees": [

    ],
    "requested_reviewers": [

    ],
    "requested_teams": [

    ],
    "labels": [

    ],
    "milestone": null,
    "draft": false,
    "commits_url": "https://api.github.com/repos/fho/testrepo/pulls/1/commits",
    "review_comments_url": "https://api.github.com/repos/fho/testrepo/pulls/1/comments",
    "review_comment_url": "https://api.github.com/repos/fho/testrepo/pulls/comments{/number}",
    "comments_url": "https://api.github.com/repos/fho/testrepo/issues/1/comments",
    "statuses_url": "https://api.github.com/repos/fho/testrepo/statuses/8ad9dec4298f6b8f020997373cf4fe22005f2c06",
    "head": {
      "label": "fho:pr",
      "ref": "pr",
      "sha": "8ad9dec4298f6b8f020997373cf4fe22005f2c06",
      "user": {
        "login": "fho",
        "id": 514535,
        "node_id": "MDQ6VXNlcjUxNDUzNQ==",
        "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
        "gravatar_id": "",
        "url": "https://api.github.com/users/fho",
        "html_url": "https://github.com/fho",
        "followers_url": "https://api.github.com/users/fho/followers",
        "following_url": "https://api.github.com/users/fho/following{/other_user}",
        "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
        "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
        "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
        "organizations_url": "https://api.github.com/users/fho/orgs",
        "repos_url": "https://api.github.com/users/fho/repos",
        "events_url": "https://api.github.com/users/fho/events{/privacy}",
        "received_events_url": "https://api.github.com/users/fho/received_events",
        "type": "User",
        "site_admin": false
      },
      "repo": {
        "id": 366295491,
        "node_id": "MDEwOlJlcG9zaXRvcnkzNjYyOTU0OTE=",
        "name": "testrepo",
        "full_name": "fho/testrepo",
        "private": false,
        "owner": {
          "login": "fho",
          "id": 514535,
          "node_id": "MDQ6VXNlcjUxNDUzNQ==",
          "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/fho",
          "html_url": "https://github.com/fho",
          "followers_url": "https://api.github.com/users/fho/followers",
          "following_url": "https://api.github.com/users/fho/following{/other_user}",
          "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
          "organizations_url": "https://api.github.com/users/fho/orgs",
          "repos_url": "https://api.github.com/users/fho/repos",
          "events_url": "https://api.github.com/users/fho/events{/privacy}",
          "received_events_url": "https://api.github.com/users/fho/received_events",
          "type": "User",
          "site_admin": false
        },
        "html_url": "https://github.com/fho/testrepo",
        "description": null,
        "fork": false,
        "url": "https://api.github.com/repos/fho/testrepo",
        "forks_url": "https://api.github.com/repos/fho/testrepo/forks",
        "keys_url": "https://api.github.com/repos/fho/testrepo/keys{/key_id}",
        "collaborators_url": "https://api.github.com/repos/fho/testrepo/collaborators{/collaborator}",
        "teams_url": "https://api.github.com/repos/fho/testrepo/teams",
        "hooks_url": "https://api.github.com/repos/fho/testrepo/hooks",
        "issue_events_url": "https://api.github.com/repos/fho/testrepo/issues/events{/number}",
        "events_url": "https://api.github.com/repos/fho/testrepo/events",
        "assignees_url": "https://api.github.com/repos/fho/testrepo/assignees{/user}",
        "branches_url": "https://api.github.com/repos/fho/testrepo/branches{/branch}",
        "tags_url": "https://api.github.com/repos/fho/testrepo/tags",
        "blobs_url": "https://api.github.com/repos/fho/testrepo/git/blobs{/sha}",
        "git_tags_url": "https://api.github.com/repos/fho/testrepo/git/tags{/sha}",
        "git_refs_url": "https://api.github.com/repos/fho/testrepo/git/refs{/sha}",
        "trees_url": "https://api.github.com/repos/fho/testrepo/git/trees{/sha}",
        "statuses_url": "https://api.github.com/repos/fho/testrepo/statuses/{sha}",
        "languages_url": "https://api.github.com/repos/fho/testrepo/languages",
        "stargazers_url": "https://api.github.com/repos/fho/testrepo/stargazers",
        "contributors_url": "https://api.github.com/repos/fho/testrepo/contributors",
        "subscribers_url": "https://api.github.com/repos/fho/testrepo/subscribers",
        "subscription_url": "https://api.github.com/repos/fho/testrepo/subscription",
        "commits_url": "https://api.github.com/repos/fho/testrepo/commits{/sha}",
        "git_commits_url": "https://api.github.com/repos/fho/testrepo/git/commits{/sha}",
        "comments_url": "https://api.github.com/repos/fho/testrepo/comments{/number}",
        "issue_comment_url": "https://api.github.com/repos/fho/testrepo/issues/comments{/number}",
        "contents_url": "https://api.github.com/repos/fho/testrepo/contents/{+path}",
        "compare_url": "https://api.github.com/repos/fho/testrepo/compare/{base}...{head}",
        "merges_url": "https://api.github.com/repos/fho/testrepo/merges",
        "archive_url": "https://api.github.com/repos/fho/testrepo/{archive_format}{/ref}",
        "downloads_url": "https://api.github.com/repos/fho/testrepo/downloads",
        "issues_url": "https://api.github.com/repos/fho/testrepo/issues{/number}",
        "pulls_url": "https://api.github.com/repos/fho/testrepo/pulls{/number}",
        "milestones_url": "https://api.github.com/repos/fho/testrepo/milestones{/number}",
        "notifications_url": "https://api.github.com/repos/fho/testrepo/notifications{?since,all,participating}",
        "labels_url": "https://api.github.com/repos/fho/testrepo/labels{/name}",
        "releases_url": "https://api.github.com/repos/fho/testrepo/releases{/id}",
        "deployments_url": "https://api.github.com/repos/fho/testrepo/deployments",
        "created_at": "2021-05-11T07:35:03Z",
        "updated_at": "2021-05-11T07:35:13Z",
        "pushed_at": "2021-05-11T07:40:46Z",
        "git_url": "git://github.com/fho/testrepo.git",
        "ssh_url": "git@github.com:fho/testrepo.git",
        "clone_url": "https://github.com/fho/testrepo.git",
        "svn_url": "https://github.com/fho/testrepo",
        "homepage": null,
        "size": 0,
        "stargazers_count": 0,
        "watchers_count": 0,
        "language": null,
        "has_issues": true,
        "has_projects": true,
        "has_downloads": true,
        "has_wiki": true,
        "has_pages": false,
        "forks_count": 0,
        "mirror_url": null,
        "archived": false,
        "disabled": false,
        "open_issues_count": 1,
        "license": null,
        "forks": 0,
        "open_issues": 1,
        "watchers": 0,
        "default_branch": "main",
        "allow_squash_merge": true,
        "allow_merge_commit": true,
        "allow_rebase_merge": true,
        "delete_branch_on_merge": false
      }
    },
    "base": {
      "label": "fho:main",
      "ref": "main",
      "sha": "ddba3b1154ff3f186ad9ff91b17b09e88aa46644",
      "user": {
        "login": "fho",
        "id": 514535,
        "node_id": "MDQ6VXNlcjUxNDUzNQ==",
        "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
        "gravatar_id": "",
        "url": "https://api.github.com/users/fho",
        "html_url": "https://github.com/fho",
        "followers_url": "https://api.github.com/users/fho/followers",
        "following_url": "https://api.github.com/users/fho/following{/other_user}",
        "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
        "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
        "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
        "organizations_url": "https://api.github.com/users/fho/orgs",
        "repos_url": "https://api.github.com/users/fho/repos",
        "events_url": "https://api.github.com/users/fho/events{/privacy}",
        "received_events_url": "https://api.github.com/users/fho/received_events",
        "type": "User",
        "site_admin": false
      },
      "repo": {
        "id": 366295491,
        "node_id": "MDEwOlJlcG9zaXRvcnkzNjYyOTU0OTE=",
        "name": "testrepo",
        "full_name": "fho/testrepo",
        "private": false,
        "owner": {
          "login": "fho",
          "id": 514535,
          "node_id": "MDQ6VXNlcjUxNDUzNQ==",
          "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
          "gravatar_id": "",
          "url": "https://api.github.com/users/fho",
          "html_url": "https://github.com/fho",
          "followers_url": "https://api.github.com/users/fho/followers",
          "following_url": "https://api.github.com/users/fho/following{/other_user}",
          "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
          "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
          "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
          "organizations_url": "https://api.github.com/users/fho/orgs",
          "repos_url": "https://api.github.com/users/fho/repos",
          "events_url": "https://api.github.com/users/fho/events{/privacy}",
          "received_events_url": "https://api.github.com/users/fho/received_events",
          "type": "User",
          "site_admin": false
        },
        "html_url": "https://github.com/fho/testrepo",
        "description": null,
        "fork": false,
        "url": "https://api.github.com/repos/fho/testrepo",
        "forks_url": "https://api.github.com/repos/fho/testrepo/forks",
        "keys_url": "https://api.github.com/repos/fho/testrepo/keys{/key_id}",
        "collaborators_url": "https://api.github.com/repos/fho/testrepo/collaborators{/collaborator}",
        "teams_url": "https://api.github.com/repos/fho/testrepo/teams",
        "hooks_url": "https://api.github.com/repos/fho/testrepo/hooks",
        "issue_events_url": "https://api.github.com/repos/fho/testrepo/issues/events{/number}",
        "events_url": "https://api.github.com/repos/fho/testrepo/events",
        "assignees_url": "https://api.github.com/repos/fho/testrepo/assignees{/user}",
        "branches_url": "https://api.github.com/repos/fho/testrepo/branches{/branch}",
        "tags_url": "https://api.github.com/repos/fho/testrepo/tags",
        "blobs_url": "https://api.github.com/repos/fho/testrepo/git/blobs{/sha}",
        "git_tags_url": "https://api.github.com/repos/fho/testrepo/git/tags{/sha}",
        "git_refs_url": "https://api.github.com/repos/fho/testrepo/git/refs{/sha}",
        "trees_url": "https://api.github.com/repos/fho/testrepo/git/trees{/sha}",
        "statuses_url": "https://api.github.com/repos/fho/testrepo/statuses/{sha}",
        "languages_url": "https://api.github.com/repos/fho/testrepo/languages",
        "stargazers_url": "https://api.github.com/repos/fho/testrepo/stargazers",
        "contributors_url": "https://api.github.com/repos/fho/testrepo/contributors",
        "subscribers_url": "https://api.github.com/repos/fho/testrepo/subscribers",
        "subscription_url": "https://api.github.com/repos/fho/testrepo/subscription",
        "commits_url": "https://api.github.com/repos/fho/testrepo/commits{/sha}",
        "git_commits_url": "https://api.github.com/repos/fho/testrepo/git/commits{/sha}",
        "comments_url": "https://api.github.com/repos/fho/testrepo/comments{/number}",
        "issue_comment_url": "https://api.github.com/repos/fho/testrepo/issues/comments{/number}",
        "contents_url": "https://api.github.com/repos/fho/testrepo/contents/{+path}",
        "compare_url": "https://api.github.com/repos/fho/testrepo/compare/{base}...{head}",
        "merges_url": "https://api.github.com/repos/fho/testrepo/merges",
        "archive_url": "https://api.github.com/repos/fho/testrepo/{archive_format}{/ref}",
        "downloads_url": "https://api.github.com/repos/fho/testrepo/downloads",
        "issues_url": "https://api.github.com/repos/fho/testrepo/issues{/number}",
        "pulls_url": "https://api.github.com/repos/fho/testrepo/pulls{/number}",
        "milestones_url": "https://api.github.com/repos/fho/testrepo/milestones{/number}",
        "notifications_url": "https://api.github.com/repos/fho/testrepo/notifications{?since,all,participating}",
        "labels_url": "https://api.github.com/repos/fho/testrepo/labels{/name}",
        "releases_url": "https://api.github.com/repos/fho/testrepo/releases{/id}",
        "deployments_url": "https://api.github.com/repos/fho/testrepo/deployments",
        "created_at": "2021-05-11T07:35:03Z",
        "updated_at": "2021-05-11T07:35:13Z",
        "pushed_at": "2021-05-11T07:40:46Z",
        "git_url": "git://github.com/fho/testrepo.git",
        "ssh_url": "git@github.com:fho/testrepo.git",
        "clone_url": "https://github.com/fho/testrepo.git",
        "svn_url": "https://github.com/fho/testrepo",
        "homepage": null,
        "size": 0,
        "stargazers_count": 0,
        "watchers_count": 0,
        "language": null,
        "has_issues": true,
        "has_projects": true,
        "has_downloads": true,
        "has_wiki": true,
        "has_pages": false,
        "forks_count": 0,
        "mirror_url": null,
        "archived": false,
        "disabled": false,
        "open_issues_count": 1,
        "license": null,
        "forks": 0,
        "open_issues": 1,
        "watchers": 0,
        "default_branch": "main",
        "allow_squash_merge": true,
        "allow_merge_commit": true,
        "allow_rebase_merge": true,
        "delete_branch_on_merge": false
      }
    },
    "_links": {
      "self": {
        "href": "https://api.github.com/repos/fho/testrepo/pulls/1"
      },
      "html": {
        "href": "https://github.com/fho/testrepo/pull/1"
      },
      "issue": {
        "href": "https://api.github.com/repos/fho/testrepo/issues/1"
      },
      "comments": {
        "href": "https://api.github.com/repos/fho/testrepo/issues/1/comments"
      },
      "review_comments": {
        "href": "https://api.github.com/repos/fho/testrepo/pulls/1/comments"
      },
      "review_comment": {
        "href": "https://api.github.com/repos/fho/testrepo/pulls/comments{/number}"
      },
      "commits": {
        "href": "https://api.github.com/repos/fho/testrepo/pulls/1/commits"
      },
      "statuses": {
        "href": "https://api.github.com/repos/fho/testrepo/statuses/8ad9dec4298f6b8f020997373cf4fe22005f2c06"
      }
    },
    "author_association": "OWNER",
    "auto_merge": null,
    "active_lock_reason": null,
    "merged": false,
    "mergeable": null,
    "rebaseable": null,
    "mergeable_state": "unknown",
    "merged_by": null,
    "comments": 0,
    "review_comments": 0,
    "maintainer_can_modify": false,
    "commits": 2,
    "additions": 3,
    "deletions": 0,
    "changed_files": 1
  },
  "before": "7747978f08eae3c4308b8ce2806801f405d6bbd2",
  "after": "8ad9dec4298f6b8f020997373cf4fe22005f2c06",
  "repository": {
    "id": 366295491,
    "node_id": "MDEwOlJlcG9zaXRvcnkzNjYyOTU0OTE=",
    "name": "testrepo",
    "full_name": "fho/testrepo",
    "private": false,
    "owner": {
      "login": "fho",
      "id": 514535,
      "node_id": "MDQ6VXNlcjUxNDUzNQ==",
      "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
      "gravatar_id": "",
      "url": "https://api.github.com/users/fho",
      "html_url": "https://github.com/fho",
      "followers_url": "https://api.github.com/users/fho/followers",
      "following_url": "https://api.github.com/users/fho/following{/other_user}",
      "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
      "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
      "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
      "organizations_url": "https://api.github.com/users/fho/orgs",
      "repos_url": "https://api.github.com/users/fho/repos",
      "events_url": "https://api.github.com/users/fho/events{/privacy}",
      "received_events_url": "https://api.github.com/users/fho/received_events",
      "type": "User",
      "site_admin": false
    },
    "html_url": "https://github.com/fho/testrepo",
    "description": null,
    "fork": false,
    "url": "https://api.github.com/repos/fho/testrepo",
    "forks_url": "https://api.github.com/repos/fho/testrepo/forks",
    "keys_url": "https://api.github.com/repos/fho/testrepo/keys{/key_id}",
    "collaborators_url": "https://api.github.com/repos/fho/testrepo/collaborators{/collaborator}",
    "teams_url": "https://api.github.com/repos/fho/testrepo/teams",
    "hooks_url": "https://api.github.com/repos/fho/testrepo/hooks",
    "issue_events_url": "https://api.github.com/repos/fho/testrepo/issues/events{/number}",
    "events_url": "https://api.github.com/repos/fho/testrepo/events",
    "assignees_url": "https://api.github.com/repos/fho/testrepo/assignees{/user}",
    "branches_url": "https://api.github.com/repos/fho/testrepo/branches{/branch}",
    "tags_url": "https://api.github.com/repos/fho/testrepo/tags",
    "blobs_url": "https://api.github.com/repos/fho/testrepo/git/blobs{/sha}",
    "git_tags_url": "https://api.github.com/repos/fho/testrepo/git/tags{/sha}",
    "git_refs_url": "https://api.github.com/repos/fho/testrepo/git/refs{/sha}",
    "trees_url": "https://api.github.com/repos/fho/testrepo/git/trees{/sha}",
    "statuses_url": "https://api.github.com/repos/fho/testrepo/statuses/{sha}",
    "languages_url": "https://api.github.com/repos/fho/testrepo/languages",
    "stargazers_url": "https://api.github.com/repos/fho/testrepo/stargazers",
    "contributors_url": "https://api.github.com/repos/fho/testrepo/contributors",
    "subscribers_url": "https://api.github.com/repos/fho/testrepo/subscribers",
    "subscription_url": "https://api.github.com/repos/fho/testrepo/subscription",
    "commits_url": "https://api.github.com/repos/fho/testrepo/commits{/sha}",
    "git_commits_url": "https://api.github.com/repos/fho/testrepo/git/commits{/sha}",
    "comments_url": "https://api.github.com/repos/fho/testrepo/comments{/number}",
    "issue_comment_url": "https://api.github.com/repos/fho/testrepo/issues/comments{/number}",
    "contents_url": "https://api.github.com/repos/fho/testrepo/contents/{+path}",
    "compare_url": "https://api.github.com/repos/fho/testrepo/compare/{base}...{head}",
    "merges_url": "https://api.github.com/repos/fho/testrepo/merges",
    "archive_url": "https://api.github.com/repos/fho/testrepo/{archive_format}{/ref}",
    "downloads_url": "https://api.github.com/repos/fho/testrepo/downloads",
    "issues_url": "https://api.github.com/repos/fho/testrepo/issues{/number}",
    "pulls_url": "https://api.github.com/repos/fho/testrepo/pulls{/number}",
    "milestones_url": "https://api.github.com/repos/fho/testrepo/milestones{/number}",
    "notifications_url": "https://api.github.com/repos/fho/testrepo/notifications{?since,all,participating}",
    "labels_url": "https://api.github.com/repos/fho/testrepo/labels{/name}",
    "releases_url": "https://api.github.com/repos/fho/testrepo/releases{/id}",
    "deployments_url": "https://api.github.com/repos/fho/testrepo/deployments",
    "created_at": "2021-05-11T07:35:03Z",
    "updated_at": "2021-05-11T07:35:13Z",
    "pushed_at": "2021-05-11T07:40:46Z",
    "git_url": "git://github.com/fho/testrepo.git",
    "ssh_url": "git@github.com:fho/testrepo.git",
    "clone_url": "https://github.com/fho/testrepo.git",
    "svn_url": "https://github.com/fho/testrepo",
    "homepage": null,
    "size": 0,
    "stargazers_count": 0,
    "watchers_count": 0,
    "language": null,
    "has_issues": true,
    "has_projects": true,
    "has_downloads": true,
    "has_wiki": true,
    "has_pages": false,
    "forks_count": 0,
    "mirror_url": null,
    "archived": false,
    "disabled": false,
    "open_issues_count": 1,
    "license": null,
    "forks": 0,
    "open_issues": 1,
    "watchers": 0,
    "default_branch": "main"
  },
  "sender": {
    "login": "fho",
    "id": 514535,
    "node_id": "MDQ6VXNlcjUxNDUzNQ==",
    "avatar_url": "https://avatars.githubusercontent.com/u/514535?v=4",
    "gravatar_id": "",
    "url": "https://api.github.com/users/fho",
    "html_url": "https://github.com/fho",
    "followers_url": "https://api.github.com/users/fho/followers",
    "following_url": "https://api.github.com/users/fho/following{/other_user}",
    "gists_url": "https://api.github.com/users/fho/gists{/gist_id}",
    "starred_url": "https://api.github.com/users/fho/starred{/owner}{/repo}",
    "subscriptions_url": "https://api.github.com/users/fho/subscriptions",
    "organizations_url": "https://api.github.com/users/fho/orgs",
    "repos_url": "https://api.github.com/users/fho/repos",
    "events_url": "https://api.github.com/users/fho/events{/privacy}",
    "received_events_url": "https://api.github.com/users/fho/received_events",
    "type": "User",
    "site_admin": false
  }
}
`

func newPullRequestSyncHTTPReq() *http.Request {
	hdrs := http.Header{}
	hdrs.Set("Content-Type", "application/json")
	hdrs.Set("X-GitHub-Delivery", "3355fab0-b22c-11eb-9936-51d9540c0cdc")
	hdrs.Set("X-GitHub-Event", "pull_request")

	return &http.Request{
		Method: http.MethodPost,
		Header: hdrs,
		Body:   io.NopCloser(strings.NewReader(pullRequestSynchronizeEventPayload)),
	}
}
