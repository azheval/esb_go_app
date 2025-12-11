package scripting

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"esb-go-app/storage"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
	"go.starlark.net/starlarkstruct"
)

// StarlarkRunner implements the Runner interface for Starlark scripts.
type StarlarkRunner struct {
	logger     *slog.Logger
	httpClient *HTTPClient // Injected HTTP client
	store      *storage.Store
}

// NewStarlarkRunner creates a new StarlarkRunner instance.
func NewStarlarkRunner(logger *slog.Logger, httpClient *HTTPClient, store *storage.Store) *StarlarkRunner {
	return &StarlarkRunner{
		logger:     logger,
		httpClient: httpClient,
		store:      store,
	}
}

// Execute runs the Starlark script.
func (r *StarlarkRunner) Execute(script string, messageBody map[string]interface{}, messageHeaders map[string]interface{}) (*TransformedMessage, error) {
	thread := &starlark.Thread{Name: "script_execution_thread"}

	// Inject logger
	logModule := starlarkstruct.FromStringDict(starlark.String("log"), starlark.StringDict{
		"info": starlark.NewBuiltin("log.info", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var msg string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
				return nil, err
			}
			r.logger.Info(msg)
			return starlark.None, nil
		}),
		"warn": starlark.NewBuiltin("log.warn", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var msg string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
				return nil, err
			}
			r.logger.Warn(msg)
			return starlark.None, nil
		}),
		"error": starlark.NewBuiltin("log.error", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var msg string
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
				return nil, err
			}
			r.logger.Error(msg)
			return starlark.None, nil
		}),
	})

	// Inject HTTP client
	httpClientModule := starlarkstruct.FromStringDict(starlark.String("http"), starlark.StringDict{
		"get": starlark.NewBuiltin("http.get", func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			var url string
			var headersDict *starlark.Dict
			if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url, "headers?", &headersDict); err != nil {
				return nil, err
			}
			headers := make(map[string]string)
			if headersDict != nil {
				for _, item := range headersDict.Items() {
					key, _ := item.Index(0).(starlark.String)
					val, _ := item.Index(1).(starlark.String)
					headers[key.GoString()] = val.GoString()
				}
			}
			resp := r.httpClient.Get(url, headers)
			return starlarkstruct.FromStringDict(starlark.String("HTTPResponse"), starlark.StringDict{
				"status_code": starlark.MakeInt(resp.StatusCode),
				"body":        starlark.String(resp.Body),
				"headers":     convertStringMapToStarlarkDict(resp.Headers),
				"error":       starlark.String(resp.Error),
			}), nil
		}),
	})

	starlarkBody, err := convertMapToStarlarkDict(messageBody)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message body to Starlark dict: %w", err)
	}
	starlarkHeaders, err := convertMapToStarlarkDict(messageHeaders)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message headers to Starlark dict: %w", err)
	}

	predeclared := starlark.StringDict{
		"log":  logModule,
		"http": httpClientModule,
		"json": starlarkjson.Module,
	}

	starlarkGlobals, err := starlark.ExecFile(thread, "script", script, predeclared)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Starlark script: %w", err)
	}

	if transformFunc, found := starlarkGlobals["transform"]; found {
		if callable, ok := transformFunc.(starlark.Callable); ok {
			args := starlark.Tuple{starlarkBody, starlarkHeaders}
			result, err := starlark.Call(thread, callable, args, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to execute transform function: %w", err)
			}

			if result == starlark.None {
				return nil, nil // Message filtered
			}

			resultMap, err := convertStarlarkDictToMap(result)
			if err != nil {
				return nil, fmt.Errorf("transform result must be a dict, got %s", result.Type())
			}

			transformedBody, _ := resultMap["body"].(map[string]interface{})
			if transformedBody == nil {
				return nil, nil // No body returned, treat as filtered
			}

			return &TransformedMessage{
				Body:    transformedBody,
				Headers: messageHeaders, // Headers passed through
			}, nil
		}
	}

	if collectFunc, found := starlarkGlobals["collect"]; found {
		if callable, ok := collectFunc.(starlark.Callable); ok {
			result, err := starlark.Call(thread, callable, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to execute collect function: %w", err)
			}
			if result == starlark.None {
				return nil, nil // No data collected
			}
			resultMap, err := convertStarlarkDictToMap(result)
			if err != nil {
				return nil, fmt.Errorf("collect result must be a dict, got %s", result.Type())
			}
			if len(resultMap) > 0 {
				return &TransformedMessage{
					Body:    resultMap,
					Headers: make(map[string]interface{}),
				}, nil
			}
			return nil, nil
		}
	}

	return nil, fmt.Errorf("script must define a 'transform' or 'collect' function")
}

// convertMapToStarlarkDict converts a Go map[string]interface{} to a Starlark dictionary.
func convertMapToStarlarkDict(goMap map[string]interface{}) (*starlark.Dict, error) {
	dict := starlark.NewDict(len(goMap))
	if goMap == nil {
		return dict, nil
	}
	for k, v := range goMap {
		starlarkKey := starlark.String(k)
		starlarkValue, err := toStarlarkValue(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert value for key %s: %w", k, err)
		}
		if err := dict.SetKey(starlarkKey, starlarkValue); err != nil {
			return nil, fmt.Errorf("failed to set key %s in Starlark dict: %w", k, err)
		}
	}
	return dict, nil
}

// toStarlarkValue converts a Go interface{} to a Starlark value.
func toStarlarkValue(v interface{}) (starlark.Value, error) {
	switch val := v.(type) {
	case nil:
		return starlark.None, nil
	case bool:
		return starlark.Bool(val), nil
	case string:
		return starlark.String(val), nil
	case int:
		return starlark.MakeInt(val), nil
	case int64:
		return starlark.MakeInt64(val), nil
	case float64:
		return starlark.Float(val), nil
	case map[string]interface{}:
		return convertMapToStarlarkDict(val)
	case []interface{}:
		list := make([]starlark.Value, len(val))
		for i, item := range val {
			starlarkItem, err := toStarlarkValue(item)
			if err != nil {
				return nil, err
			}
			list[i] = starlarkItem
		}
		return starlark.NewList(list), nil
	default:
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("unsupported type %T for Starlark conversion", v)
		}
		var genericMap interface{}
		if err := json.Unmarshal(jsonBytes, &genericMap); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON into generic map for Starlark conversion: %w", err)
		}
		return toStarlarkValue(genericMap)
	}
}

// convertStarlarkDictToMap converts a Starlark dictionary to a Go map[string]interface{}.
func convertStarlarkDictToMap(starlarkVal starlark.Value) (map[string]interface{}, error) {
	goMap := make(map[string]interface{})
	if dict, ok := starlarkVal.(*starlark.Dict); ok {
		for _, item := range dict.Items() {
			key, ok := item.Index(0).(starlark.String)
			if !ok {
				return nil, fmt.Errorf("starlark dict key must be string, got %T", item.Index(0))
			}
			val := item.Index(1)
			goVal, err := fromStarlarkValue(val)
			if err != nil {
				return nil, fmt.Errorf("failed to convert value for key %s: %w", key.GoString(), err)
			}
			goMap[key.GoString()] = goVal
		}
	} else if structVal, ok := starlarkVal.(*starlarkstruct.Struct); ok {
		for _, field := range structVal.AttrNames() {
			val, err := structVal.Attr(field)
			if err != nil {
				return nil, fmt.Errorf("failed to get field %s from Starlark struct: %w", field, err)
			}
			goVal, err := fromStarlarkValue(val)
			if err != nil {
				return nil, err
			}
			goMap[field] = goVal
		}
	} else {
		return nil, fmt.Errorf("cannot convert Starlark value of type %T to Go map", starlarkVal)
	}
	return goMap, nil
}

// fromStarlarkValue converts a Starlark value to a Go interface{}.
func fromStarlarkValue(s starlark.Value) (interface{}, error) {
	switch v := s.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.String:
		return v.GoString(), nil
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("starlark.Int too large for int64")
		}
		return i, nil
	case starlark.Float:
		return float64(v), nil
	case *starlark.Dict:
		return convertStarlarkDictToMap(v)
	case *starlark.List:
		goList := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := fromStarlarkValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			goList[i] = item
		}
		return goList, nil
	case *starlark.Tuple:
		goList := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := fromStarlarkValue(v.Index(i))
			if err != nil {
				return nil, err
			}
			goList[i] = item
		}
		return goList, nil
	case *starlarkstruct.Struct: // Handle structs for HTTPResponse etc.
		goMap := make(map[string]interface{})
		for _, field := range v.AttrNames() {
			val, err := v.Attr(field)
			if err != nil {
				return nil, fmt.Errorf("failed to get field %s from Starlark struct: %w", field, err)
			}
			goVal, err := fromStarlarkValue(val)
			if err != nil {
				return nil, err
			}
			goMap[field] = goVal
		}
		return goMap, nil
	default:
		return nil, fmt.Errorf("unsupported Starlark type for conversion: %T", v)
	}
}

// convertStringMapToStarlarkDict converts a Go map[string]string to a Starlark dictionary.
func convertStringMapToStarlarkDict(goMap map[string]string) *starlark.Dict {
	dict := starlark.NewDict(len(goMap))
	for k, v := range goMap {
		dict.SetKey(starlark.String(k), starlark.String(v))
	}
	return dict
}
