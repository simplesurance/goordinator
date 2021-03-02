package goordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/itchyny/gojq"
	"github.com/simplesurance/goordinator/internal/action"
	"github.com/simplesurance/goordinator/internal/action/httprequest"
	"github.com/simplesurance/goordinator/internal/cfg"
	"github.com/simplesurance/goordinator/internal/provider"
	"github.com/simplesurance/goordinator/internal/stringutils"
)

// ActionConfig is an interface for an action that is executed as part of a Rule.
type ActionConfig interface {
	// Template runs templateFn for all configuration options of the action
	// that should be templated and returns a runnable action.
	Template(templateFn func(string) (string, error)) (action.Runner, error)
	// String returns a short representation of the ActionConfig
	String() string
	// String returns a formatted detailed description.
	DetailedString() string
}

// Rule defines the condition that must apply for an event and the actions that
// are run when conditions match.
type Rule struct {
	name        string
	eventSource string
	filterQuery *gojq.Query
	actions     []ActionConfig
}

func NewRule(name, eventProvider, jqQuery string, actions []ActionConfig) (*Rule, error) {
	query, err := gojq.Parse(jqQuery)
	if err != nil {
		return nil, err
	}

	return &Rule{
		name:        name,
		eventSource: eventProvider,
		filterQuery: query,
		actions:     actions,
	}, nil
}

// Match returns Match if the event.Provider matches the Rule.EventSource and
// the filter-query of the rule evalutes to true for JSON representation of the event.
func (r *Rule) Match(ctx context.Context, event *provider.Event) (MatchResult, error) {
	var evUn interface{}

	if r.eventSource != event.Provider {
		return EventSourceMismatch, nil
	}

	err := json.Unmarshal(event.Json, &evUn)
	if err != nil {
		return MatchResultUndefined, fmt.Errorf("unmarshaling json failed: %w", err)
	}

	iter := r.filterQuery.RunWithContext(ctx, evUn)

	result, ok := iter.Next()
	if !ok {
		return MatchResultUndefined, fmt.Errorf("json query returned 0 results, query: %q", r.filterQuery.String())
	}

	if _, ok := iter.Next(); ok {
		return MatchResultUndefined, fmt.Errorf("json query returned multiple results, expected 1, query: %q", r.filterQuery.String())
	}

	switch val := result.(type) {
	case error:
		return MatchResultUndefined, val

	case bool:
		if val {
			return Match, nil
		}

		return RuleMismatch, nil

	default:
		return MatchResultUndefined, fmt.Errorf(
			"json query returned non-bool result: %+v (%T), query: %q",
			result, result, r.filterQuery.String(),
		)
	}
}

// TemplateActions templates the filter query of the rule for the specific event.
func (r *Rule) TemplateActions(ctx context.Context, event *provider.Event) ([]action.Runner, error) {
	result := make([]action.Runner, 0, len(r.actions))

	for _, actionDef := range r.actions {
		templFun := func(in string) (string, error) {
			templ, err := template.New("action").Parse(in)
			if err != nil {
				return "", err
			}

			var out bytes.Buffer

			templateContext := struct{ Event *provider.Event }{
				Event: event,
			}

			err = templ.Execute(&out, &templateContext)
			if err != nil {
				return "", err
			}

			return out.String(), nil
		}

		runner, err := actionDef.Template(templFun)
		if err != nil {
			return nil, fmt.Errorf("templating action definition %q failed: %w", actionDef, err)
		}

		result = append(result, runner)
	}

	return result, nil
}

// RulesFromCfg instantiates Rules from a rulesCfg configuration.
func RulesFromCfg(cfg *cfg.RulesCfg) (Rules, error) {
	result := make([]*Rule, 0, len(cfg.Rules))

	for _, cfgRule := range cfg.Rules {
		var actions []ActionConfig

		// TODO: ensure names are unique
		if cfgRule.Name == "" {
			return nil, errors.New("rule: missing field: 'name'")
		}

		if len(cfgRule.Actions) == 0 {
			return nil, fmt.Errorf("rule %s: missing array field: 'action'", cfgRule.Name)
		}

		for _, cfgAction := range cfgRule.Actions {
			val, ok := cfgAction["action"]
			if !ok {
				return nil, fmt.Errorf("rule %s: action: missing string field 'action'", cfgRule.Name)
			}

			actionName, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("rule %s: action: action field is not a string field", cfgRule.Name)
			}

			switch strings.ToLower(actionName) {
			case "httprequest":
				cfg, err := httprequest.NewConfigFromMap(cfgAction)
				if err != nil {
					return nil, fmt.Errorf("rule %s: action %s: httprequest: parsing  failed: %w", cfgRule.Name, actionName, err)
				}

				actions = append(actions, cfg)

			default:
				return nil, fmt.Errorf("rule %s: unsupported action: %q", cfgRule.Name, actionName)
			}
		}

		rule, err := NewRule(cfgRule.Name, cfgRule.EventSource, cfgRule.FilterQuery, actions)
		if err != nil {
			return nil, err
		}

		result = append(result, rule)
	}

	return result, nil
}

func (r *Rule) String() string {
	return r.name
}

func (r *Rule) DetailedString() string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("Name: %s\nEventSource: %s\nFilterQuery: %s\n", r.name, r.eventSource, r.filterQuery))

	for i, action := range r.actions {
		if i == 0 {
			result.WriteString("Actions:\n")
		}

		result.WriteString(fmt.Sprintf("%s\n", stringutils.IndentString(action.DetailedString(), "")))
	}

	return result.String()
}
