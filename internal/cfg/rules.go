package cfg

import (
	"io"
	"io/ioutil"

	"github.com/BurntSushi/toml"
)

type RulesCfg struct {
	Rules []*Rules `toml:"rule"`
}

type Rules struct {
	Name        string                   `toml:"name"`
	EventSource string                   `toml:"event_source"`
	FilterQuery string                   `toml:"filter_query"`
	Actions     []map[string]interface{} `toml:"action"`
}

func LoadRules(reader io.Reader) (*RulesCfg, error) {
	var result RulesCfg

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if err := toml.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (r *RulesCfg) Marshal(writer io.Writer) error {
	return toml.NewEncoder(writer).Encode(r)
}
