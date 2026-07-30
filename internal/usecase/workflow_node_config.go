package usecase

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const (
	maxWorkflowDelaySeconds = 7 * 24 * 60 * 60
	maxWorkflowQueueLength  = 64
)

type workflowTaskConfig struct {
	Queue          string          `json:"queue"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	MaxRetries     int             `json:"max_retries"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

type workflowWebhookConfig struct {
	EndpointID string          `json:"endpoint_id"`
	Payload    json.RawMessage `json:"payload"`
}

type workflowDelayConfig struct {
	DurationSeconds int `json:"duration_seconds"`
}

type workflowConditionConfig struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

type workflowBranchConfig struct {
	Branch *bool `json:"branch"`
}

func decodeWorkflowNodeConfig(config map[string]any, destination any) error {
	if config == nil {
		return errors.New("config must be an object")
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("config must be a single object")
	}
	return nil
}

func parseWorkflowTaskConfig(config map[string]any) (workflowTaskConfig, error) {
	var result workflowTaskConfig
	if err := decodeWorkflowNodeConfig(config, &result); err != nil {
		return result, err
	}
	result.Queue = strings.TrimSpace(result.Queue)
	if result.Queue == "" || len(result.Queue) > maxWorkflowQueueLength {
		return result, errors.New("queue must be between 1 and 64 characters")
	}
	if result.Priority < 0 || result.Priority > 9 {
		return result, errors.New("priority must be between 0 and 9")
	}
	if result.MaxRetries < 0 || result.MaxRetries > 99 {
		return result, errors.New("max_retries must be between 0 and 99")
	}
	if result.TimeoutSeconds < 0 || result.TimeoutSeconds > 86400 {
		return result, errors.New("timeout_seconds must be between 0 and 86400")
	}
	if len(result.Payload) > 0 && !json.Valid(result.Payload) {
		return result, errors.New("payload must be valid JSON")
	}
	return result, nil
}

func parseWorkflowWebhookConfig(config map[string]any) (workflowWebhookConfig, error) {
	var result workflowWebhookConfig
	if err := decodeWorkflowNodeConfig(config, &result); err != nil {
		return result, err
	}
	result.EndpointID = strings.TrimSpace(result.EndpointID)
	if result.EndpointID == "" {
		return result, errors.New("endpoint_id is required")
	}
	if len(result.Payload) > 0 && !json.Valid(result.Payload) {
		return result, errors.New("payload must be valid JSON")
	}
	return result, nil
}

func parseWorkflowDelayConfig(config map[string]any) (workflowDelayConfig, error) {
	var result workflowDelayConfig
	if err := decodeWorkflowNodeConfig(config, &result); err != nil {
		return result, err
	}
	if result.DurationSeconds < 0 || result.DurationSeconds > maxWorkflowDelaySeconds {
		return result, fmt.Errorf("duration_seconds must be between 0 and %d", maxWorkflowDelaySeconds)
	}
	return result, nil
}

func parseWorkflowConditionConfig(config map[string]any) (workflowConditionConfig, error) {
	var result workflowConditionConfig
	if err := decodeWorkflowNodeConfig(config, &result); err != nil {
		return result, err
	}
	result.Field = strings.TrimSpace(result.Field)
	result.Operator = strings.TrimSpace(result.Operator)
	if !strings.HasPrefix(result.Field, "input.") || len(result.Field) <= len("input.") {
		return result, errors.New("field must start with input.")
	}
	switch result.Operator {
	case "equals", "not_equals":
		if _, exists := config["value"]; !exists {
			return result, errors.New("value is required for equals and not_equals")
		}
	case "exists":
	default:
		return result, errors.New("operator must be equals, not_equals, or exists")
	}
	return result, nil
}

func parseWorkflowBranch(condition json.RawMessage) (*bool, error) {
	if len(condition) == 0 || bytes.Equal(bytes.TrimSpace(condition), []byte("null")) {
		return nil, nil
	}
	var result workflowBranchConfig
	decoder := json.NewDecoder(bytes.NewReader(condition))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || result.Branch == nil {
		return nil, errors.New("condition edge must contain a boolean branch")
	}
	return result.Branch, nil
}

func evaluateWorkflowCondition(config workflowConditionConfig, input json.RawMessage) (bool, error) {
	var root any
	if len(input) == 0 {
		root = map[string]any{}
	} else if err := json.Unmarshal(input, &root); err != nil {
		return false, err
	}
	current := root
	found := true
	for _, part := range strings.Split(strings.TrimPrefix(config.Field, "input."), ".") {
		object, ok := current.(map[string]any)
		if !ok {
			found = false
			break
		}
		current, found = object[part]
		if !found {
			break
		}
	}
	switch config.Operator {
	case "exists":
		return found, nil
	case "equals":
		return found && reflect.DeepEqual(current, config.Value), nil
	case "not_equals":
		return !found || !reflect.DeepEqual(current, config.Value), nil
	default:
		return false, errors.New("unsupported condition operator")
	}
}
