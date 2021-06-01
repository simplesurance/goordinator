package goordinator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/provider/github"
)

func TestHTTPHandlerEventParsing(t *testing.T) {
	type testcase struct {
		name string
		req  *http.Request

		expectedJSON              string
		expectedBranch            string
		expectedDeliveryID        string
		expectedProvider          string
		expectedEventType         string
		expectedPullRequestNumber int
		expectedCommitID          string
		expectedRepositoryOwner   string
		expectedBaseBranch        string
	}

	testcases := []testcase{
		{
			name:                      "pullRequestSync",
			req:                       newPullRequestSyncHTTPReq(),
			expectedJSON:              pullRequestSynchronizeEventPayload,
			expectedBranch:            "pr",
			expectedDeliveryID:        "3355fab0-b22c-11eb-9936-51d9540c0cdc",
			expectedProvider:          "github",
			expectedEventType:         "pull_request",
			expectedPullRequestNumber: 1,
			expectedCommitID:          "8ad9dec4298f6b8f020997373cf4fe22005f2c06",
			expectedRepositoryOwner:   "fho",
			expectedBaseBranch:        "main",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))

			ch := make(chan *github.Event, 1)
			provider := github.New([]chan<- *github.Event{ch})
			t.Cleanup(func() { close(ch) })

			respRecorder := httptest.NewRecorder()
			provider.HTTPHandler(respRecorder, tc.req)
			require.Equal(t, 200, respRecorder.Code)

			event := fromProviderEvent(<-ch)

			assert.Equal(t, tc.expectedJSON, string(event.JSON))
			assert.Equal(t, tc.expectedBranch, event.Branch)
			assert.Equal(t, tc.expectedDeliveryID, event.DeliveryID)
			assert.Equal(t, tc.expectedEventType, event.EventType)
			assert.Equal(t, tc.expectedProvider, event.Provider)
			assert.Equal(t, tc.expectedPullRequestNumber, event.PullRequestNr)
			assert.Equal(t, tc.expectedCommitID, event.CommitID)
			assert.Equal(t, tc.expectedRepositoryOwner, event.RepositoryOwner)
			assert.Equal(t, tc.expectedBaseBranch, event.BaseBranch)
		})
	}
}
