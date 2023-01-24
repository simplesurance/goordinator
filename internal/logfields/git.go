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

func StatusState(val string) zap.Field {
	return zap.String("github.status_state", val)
}

func ReviewDecision(val string) zap.Field {
	return zap.String("github.review_decision", val)
}

func StatusCheckRollupState(val string) zap.Field {
	return zap.String("github.status_check_rollup_state", val)
}

func CIStatusSummary(val string) zap.Field {
	return zap.String("github.ci_status_summary", val)
}

func CheckConclusion(val string) zap.Field {
	return zap.String("github.check_conclusion", val)
}

func CheckStatus(val string) zap.Field {
	return zap.String("github.check_status", val)
}

func Label(val string) zap.Field {
	return zap.String("github.label", val)
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
