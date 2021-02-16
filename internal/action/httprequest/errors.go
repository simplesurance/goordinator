package httprequest

import "fmt"

type ErrorHTTPRequest struct {
	Body   []byte
	Status int
}

func (e *ErrorHTTPRequest) Error() string {
	return fmt.Sprintf("http request failed with StatusCode: %d, response: %q", e.Status, string(e.Body))
}
