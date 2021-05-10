package github

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/simplesurance/goordinator/internal/provider"
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
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))

			evChan := make(chan *provider.Event, 1)
			t.Cleanup(func() { close(evChan) })

			provider := New(evChan)

			respRecorder := httptest.NewRecorder()
			provider.HTTPHandler(respRecorder, tc.req)
			require.Equal(t, 200, respRecorder.Code)

			event := <-evChan

			assert.Equal(t, tc.expectedJSON, string(event.JSON))
			assert.Equal(t, tc.expectedBranch, event.Branch)
			assert.Equal(t, tc.expectedDeliveryID, event.DeliveryID)
			assert.Equal(t, tc.expectedEventType, event.EventType)
			assert.Equal(t, tc.expectedProvider, event.Provider)
			assert.Equal(t, tc.expectedPullRequestNumber, event.PullRequestNr)
			assert.Equal(t, tc.expectedCommitID, event.CommitID)
		})
	}
}
