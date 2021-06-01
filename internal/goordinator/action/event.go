package action

type Event interface {
	GetRepository() string
	GetRepositoryOwner() string
	GetPullRequestNr() int
}
