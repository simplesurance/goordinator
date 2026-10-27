package httprequest

import "fmt"

const maxErrorBodyLen = 512

type ErrorHTTPRequest struct {
	Body   []byte
	Status int
}

func (e *ErrorHTTPRequest) Error() string {
	body := string(e.Body)
	if len(body) > maxErrorBodyLen {
		body = body[:maxErrorBodyLen] + "...(truncated)"
	}

	return fmt.Sprintf("http request failed with StatusCode: %d, response: %q", e.Status, body)
}
