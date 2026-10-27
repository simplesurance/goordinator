package httprequest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorHTTPRequestTruncatesLargeBody(t *testing.T) {
	shortErr := &ErrorHTTPRequest{
		Status: http.StatusNotFound,
		Body:   []byte("not found"),
	}
	assert.Equal(t, `http request failed with StatusCode: 404, response: "not found"`, shortErr.Error())

	largeBody := strings.Repeat("a", maxErrorBodyLen+100)
	longErr := &ErrorHTTPRequest{
		Status: http.StatusNotFound,
		Body:   []byte(largeBody),
	}
	expectedBody := strings.Repeat("a", maxErrorBodyLen) + "...(truncated)"
	assert.Equal(t, `http request failed with StatusCode: 404, response: "`+expectedBody+`"`, longErr.Error())
}
