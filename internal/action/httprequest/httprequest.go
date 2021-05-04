package httprequest

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/logfields"
)

// Runner executes a http request.
type Runner struct {
	*Config
}

func (h *Runner) String() string {
	return h.Config.String()
}

// Run sends the http request.
// It returns an ErrorHTTPRequest if request related error happens.
func (h *Runner) Run(ctx context.Context) error {
	logger := h.logger.With(h.LogFields()...)

	req, err := http.NewRequestWithContext(ctx, h.method, h.url, nil)
	if err != nil {
		return err
	}

	if h.data != "" {
		req.Body = ioutil.NopCloser(bytes.NewBufferString(h.data))
	}

	req.SetBasicAuth(h.user, h.password)
	for k, v := range h.headers {
		req.Header.Add(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Warn(
			"reading http response body failed",
			logfields.Event("http_post_reading_response_body_failed"),
			zap.Int("http_response_code", resp.StatusCode),
		)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &ErrorHTTPRequest{
			Body:   body,
			Status: resp.StatusCode,
		}
	}

	logger.Debug(
		fmt.Sprintf("http response: %s", string(body)),
		logfields.Event("http_post_request_sent"),
	)

	return nil
}

// LogFields returns fields that should be used when logging messages related
// to the action.
func (h *Runner) LogFields() []zap.Field {
	return []zap.Field{
		zap.String("action", "httprequest"),
		zap.String("http_url", h.url),
		zap.String("http_method", h.method),
	}
}
