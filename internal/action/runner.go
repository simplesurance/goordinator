package action

import (
	"context"

	"go.uber.org/zap"
)

type Runner interface {
	Run(ctx context.Context) error
	String() string
	LogFields() []zap.Field
}
