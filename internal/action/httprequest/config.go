package httprequest

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/simplesurance/goordinator/internal/action"
	"github.com/simplesurance/goordinator/internal/maputils"
	"github.com/simplesurance/goordinator/internal/provider"
)

const loggerName = "action.httprequest"

// Config is the configuration of a HTTP-Request action.
type Config struct {
	url      string
	user     string
	password string
	method   string
	headers  map[string]string
	data     string
	logger   *zap.Logger
}

// WithAuth defines user and password that is used for Basic Auth.
func WithAuth(user, password string) func(*Config) {
	return func(h *Config) {
		h.user = user
		h.password = password
	}
}

// NewConfigFromMap instantiates a config from a configuration map.
// The map is usually the unmarshalled rules configuration file.
func NewConfigFromMap(m map[string]interface{}) (*Config, error) {
	url, err := maputils.StrVal(m, "url")
	if err != nil {
		return nil, err
	}
	if url == "" {
		return nil, errors.New("url must be set")
	}

	user, err := maputils.StrVal(m, "user")
	if err != nil {
		return nil, err
	}

	password, err := maputils.StrVal(m, "password")
	if err != nil {
		return nil, err
	}

	data, err := maputils.StrVal(m, "data")
	if err != nil {
		return nil, err
	}

	method, err := maputils.StrVal(m, "method")
	if err != nil {
		return nil, err
	}

	if method == "" {
		method = "POST"
	}

	headers, err := maputils.MapSliceVal(m, "headers")
	if err != nil {
		return nil, err
	}
	strHeaders, err := maputils.ToStrMap(headers)
	if err != nil {
		return nil, fmt.Errorf("headers: %w", err)
	}

	return &Config{
		url:      url,
		user:     user,
		password: password,
		headers:  strHeaders,
		method:   method,
		data:     data,
		logger:   zap.L().Named(loggerName),
	}, nil

}

// Template runs the fn callback on all configuration options that can contain
// template strings.  fn must replace the template strings.
// It returns an executable action that uses the templated config.
func (c *Config) Template(_ *provider.Event, fn func(string) (string, error)) (action.Runner, error) {
	var err error
	newConfig := *c

	newConfig.url, err = fn(newConfig.url)
	if err != nil {
		return nil, fmt.Errorf("templating url failed: %w", err)
	}

	if newConfig.data != "" {
		newConfig.data, err = fn(newConfig.data)
		if err != nil {
			return nil, fmt.Errorf("templating data failed: %w", err)
		}
	}

	for k, v := range c.headers {
		c.headers[k], err = fn(v)
		if err != nil {
			return nil, fmt.Errorf("templating header failed: %w", err)
		}
	}

	return NewRunner(&newConfig), nil
}

func (c *Config) String() string {
	return fmt.Sprintf("httrequest: %s to %s", c.method, c.url)
}

func (c *Config) DetailedString() string {
	const maskedStr = "************"
	var result strings.Builder

	result.WriteString("http-request:\n")
	result.WriteString(fmt.Sprintf("  url: %s\n", c.url))
	result.WriteString(fmt.Sprintf("  method: %s\n", c.method))
	if c.user != "" {
		result.WriteString("  user: " + maskedStr + "\n")
	}

	if c.password != "" {
		result.WriteString("  password: " + maskedStr + "\n")
	}

	if c.data != "" {
		result.WriteString("  data: " + maskedStr + "\n")
	}

	if len(c.headers) > 0 {
		result.WriteString("  headers:\n")
	}

	for k := range c.headers {
		if c.data != "" {
			result.WriteString(fmt.Sprintf("    %s: %s\n", k, maskedStr))
		}
	}

	return result.String()
}
