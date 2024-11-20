package logfields

import "go.uber.org/zap"

func PullRequest(val int) zap.Field {
	return zap.Int("github.pull_request", val)
}

func Repository(val string) zap.Field {
	return zap.String("git.repository", val)
}

func RepositoryOwner(val string) zap.Field {
	return zap.String("github.repository_owner", val)
}

func Label(val string) zap.Field {
	return zap.String("github.label", val)
}

func Commit(val string) zap.Field {
	return zap.String("git.commit", val)
}

func BaseBranch(val string) zap.Field {
	return zap.String("git.base_branch", val)
}
