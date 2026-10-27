package githubclt

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestWrapRetryableErrorsDoesNotDuplicateRateLimitField(t *testing.T) {
	var logBuf bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(&logBuf),
		zapcore.DebugLevel,
	)

	clt := &Client{
		logger: zap.New(core),
	}

	rateLimitErr := &github.RateLimitError{
		Rate: github.Rate{
			Limit: 5000,
			Reset: github.Timestamp{Time: time.Now().Add(time.Minute)},
		},
		Message: "API rate limit exceeded",
	}

	err := clt.wrapRetryableErrors(rateLimitErr)
	require.Error(t, err)

	logLine := strings.TrimSpace(logBuf.String())
	require.NotEmpty(t, logLine)
	assert.Equal(t, 1, strings.Count(logLine, `"github_api_rate_limit":`), "logLine: %s", logLine)
}
