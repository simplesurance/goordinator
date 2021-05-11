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

		expectedJSON              []byte
		expectedBranch            string
		expectedDeliveryID        string
		expectedEventType         string
		expectedProvider          string
		expectedPullRequestNumber int
		expectedCommitID          string
	}

	testcases := []testcase{
		{
			name:                      "pullRequestSync",
			req:                       newPullRequestSyncHTTPReq(),
			expectedJSON:              []byte(pullRequestSynchronizeEventPayload),
			expectedBranch:            "pr",
			expectedDeliveryID:        "3355fab0-b22c-11eb-9936-51d9540c0cdc",
			expectedEventType:         "pull_request",
			expectedPullRequestNumber: 1,
			expectedCommitID:          "", // not parsed currently
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(zap.ReplaceGlobals(zaptest.NewLogger(t)))

			evChan := make(chan *provider.Event, 1)
			t.Cleanup(func() { close(evChan) })

			provider := New(evChan)

			respRecorder := httptest.NewRecorder()
			provider.HTTPHandler(respRecorder, newPullRequestSyncHTTPReq())
			require.Equal(t, 200, respRecorder.Code)

			event := <-evChan

			assert.Equal(t, pullRequestSynchronizeEventPayload, string(event.JSON))
			assert.Equal(t, "pr", event.Branch)
			assert.Equal(t, "3355fab0-b22c-11eb-9936-51d9540c0cdc", event.DeliveryID)
			assert.Equal(t, "pull_request", event.EventType)
			assert.Equal(t, "github", event.Provider)
			assert.Equal(t, 1, event.PullRequestNr)
		})
	}
}
