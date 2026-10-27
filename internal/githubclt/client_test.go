package githubclt

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNewClientWithAuthTokenDoesNotUseDeprecatedCancelRequest(t *testing.T) {
	clt := New("test-secret-token")

	httpClient := clt.restClt.Client()
	assert.Equal(t, DefaultHTTPClientTimeout, httpClient.Timeout)

	type requestCanceler interface {
		CancelRequest(*http.Request)
	}
	_, implementsCancelRequest := httpClient.Transport.(requestCanceler)
	assert.False(t, implementsCancelRequest, "HTTP transport must not implement deprecated CancelRequest")

	var gotAuthHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	baseURL, err := url.Parse(srv.URL + "/")
	require.NoError(t, err)
	clt.restClt.BaseURL = baseURL

	err = clt.RemoveLabel(context.Background(), "owner", "repo", 1, "label")
	require.NoError(t, err)
	assert.Equal(t, "Bearer test-secret-token", gotAuthHeader)
}

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
