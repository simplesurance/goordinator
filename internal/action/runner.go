package action

import "context"

type Runner interface {
	Run(ctx context.Context) error
	String() string
}
