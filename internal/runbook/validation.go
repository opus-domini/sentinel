package runbook

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/opus-domini/sentinel/internal/store"
)

const (
	parameterTypeString  = "string"
	parameterTypeNumber  = "number"
	parameterTypeBoolean = "boolean"
	parameterTypeSelect  = "select"
)

// ValidateDefinition validates the canonical runbook write contract shared by
// the HTTP API and MCP.
func ValidateDefinition(write store.OpsRunbookWrite) error {
	if strings.TrimSpace(write.Name) == "" {
		return fmt.Errorf("runbook name is required")
	}
	if len(write.Steps) == 0 {
		return fmt.Errorf("runbook must contain at least one step")
	}
	for index, step := range write.Steps {
		if err := validateStep(index, step); err != nil {
			return err
		}
	}
	if err := validateParameterDefinitions(write.Parameters); err != nil {
		return err
	}
	return validateWebhookURL(write.WebhookURL)
}

func validateStep(index int, step store.OpsRunbookStep) error {
	if strings.TrimSpace(step.Title) == "" {
		return fmt.Errorf("step %d: title is required", index)
	}
	if step.Timeout < 0 {
		return fmt.Errorf("step %d: timeout must not be negative", index)
	}
	if step.Retries < 0 {
		return fmt.Errorf("step %d: retries must not be negative", index)
	}
	if step.RetryDelay < 0 {
		return fmt.Errorf("step %d: retryDelay must not be negative", index)
	}
	switch step.Type {
	case stepTypeRun:
		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("step %d: command is required for type run", index)
		}
	case stepTypeScript:
		if strings.TrimSpace(step.Script) == "" {
			return fmt.Errorf("step %d: script is required for type script", index)
		}
	case stepTypeApproval:
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("step %d: description is required for type approval", index)
		}
	default:
		return fmt.Errorf("step %d: type must be run, script, or approval", index)
	}
	return nil
}

func validateParameterDefinitions(parameters []store.RunbookParameter) error {
	seen := make(map[string]struct{}, len(parameters))
	for index, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return fmt.Errorf("parameter %d: name is required", index)
		}
		if name != parameter.Name {
			return fmt.Errorf("parameter %d: name must not have surrounding whitespace", index)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("parameter %d: name %q is duplicated", index, name)
		}
		seen[name] = struct{}{}

		switch parameter.Type {
		case parameterTypeString, parameterTypeNumber, parameterTypeBoolean:
		case parameterTypeSelect:
			if len(parameter.Options) == 0 {
				return fmt.Errorf("parameter %d: select parameters require at least one option", index)
			}
			options := make(map[string]struct{}, len(parameter.Options))
			for _, rawOption := range parameter.Options {
				option := strings.TrimSpace(rawOption)
				if option == "" {
					return fmt.Errorf("parameter %d: select options must not be empty", index)
				}
				if option != rawOption {
					return fmt.Errorf("parameter %d: select options must not have surrounding whitespace", index)
				}
				if _, exists := options[option]; exists {
					return fmt.Errorf("parameter %d: select option %q is duplicated", index, option)
				}
				options[option] = struct{}{}
			}
		default:
			return fmt.Errorf("parameter %d: type must be string, number, boolean, or select", index)
		}
		if parameter.Default != "" {
			if err := validateParameterValue(parameter, parameter.Default); err != nil {
				return fmt.Errorf("parameter %d default: %w", index, err)
			}
		}
	}
	return nil
}

func validateParameterValue(parameter store.RunbookParameter, value string) error {
	switch parameter.Type {
	case parameterTypeString:
		return nil
	case parameterTypeNumber:
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("parameter %q must be a number", parameter.Name)
		}
	case parameterTypeBoolean:
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("parameter %q must be a boolean", parameter.Name)
		}
	case parameterTypeSelect:
		for _, option := range parameter.Options {
			if value == option {
				return nil
			}
		}
		return fmt.Errorf("parameter %q must be one of: %s", parameter.Name, strings.Join(parameter.Options, ", "))
	}
	return nil
}

func validateWebhookURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook URL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("webhook URL must include a host")
	}
	return nil
}

// ShellWarnings returns non-blocking shell syntax warnings for persisted
// runbook steps.
func ShellWarnings(steps []store.OpsRunbookStep) []ShellWarning {
	inputs := make([]ShellCheckInput, 0, len(steps))
	for index, step := range steps {
		switch step.Type {
		case stepTypeRun:
			inputs = append(inputs, ShellCheckInput{Step: index, Type: stepTypeRun, Source: step.Command})
		case stepTypeScript:
			inputs = append(inputs, ShellCheckInput{Step: index, Type: stepTypeScript, Source: step.Script})
		}
	}
	return ValidateShellSyntaxFromStrings(inputs)
}
