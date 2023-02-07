package githubclt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/simplesurance/goordinator/internal/goorderr"

	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestWrapRetryableErrorsGraphql(t *testing.T) {
	t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t).Named(t.Name())))

	// is the same then in vendor/github.com/shurcooL/graphql/graphql.go do()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
	}))

	t.Cleanup(srv.Close)

	clt := Client{
		logger:     zap.L(),
		graphQLClt: githubv4.NewEnterpriseClient(srv.URL, srv.Client()),
	}

	s, err := clt.ReadyForMerge(context.Background(), "test", "test", 123)
	require.Error(t, err)
	assert.Nil(t, s)

	var retryableErr *goorderr.RetryableError
	assert.ErrorAs(t, err, &retryableErr)
}

func TestWrapRetryableErrorsGraphqlWithNonStatusErr(t *testing.T) {
	err := errors.New("error")
	wrappedErr := (&Client{}).wrapGraphQLRetryableErrors(err)
	assert.Equal(t, err, wrappedErr)
}
