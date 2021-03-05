package cfg

import (
	"io"
	"io/ioutil"

	"github.com/BurntSushi/toml"
)

type Config struct {
	HttpListenAddr            string   `toml:"http_server_listen_addr"`
	HttpsListenAddr           string   `toml:"https_server_listen_addr"`
	HttpsCertFile             string   `toml:"https_ssl_cert_file"`
	HttpsKeyFile              string   `toml:"https_ssl_key_file"`
	HttpGithubWebhookEndpoint string   `toml:"github_webhook_endpoint"`
	GithubWebHookSecret       string   `toml:"github_webhook_secret"`
	LogFormat                 string   `toml:"log_format"`
	LogTimeKey                string   `toml:"log_time_key"`
	Rules                     []*Rules `toml:"rule"`
}

type Rules struct {
	Name        string                   `toml:"name"`
	EventSource string                   `toml:"event_source"`
	FilterQuery string                   `toml:"filter_query"`
	Actions     []map[string]interface{} `toml:"action"`
}

func Load(reader io.Reader) (*Config, error) {
	var result Config

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if err := toml.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *Config) Marshal(writer io.Writer) error {
	return toml.NewEncoder(writer).Encode(r)
}
