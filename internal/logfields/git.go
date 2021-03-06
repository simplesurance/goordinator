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

func BaseBranch(val string) zap.Field {
	return zap.String("git.base_branch", val)
}

func Commit(val string) zap.Field {
	return zap.String("git.commit", val)
}

func Branch(val string) zap.Field {
	return zap.String("git.branch", val)
}

func CheckStatus(val string) zap.Field {
	return zap.String("git.check_status", val)
}
