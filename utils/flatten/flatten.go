package flatten

import (
	"fmt"
	"maps"
	"reflect"
)

// FlattenJSON flattens a nested JSON into a map[string]string.
func FlattenJSON(data map[string]any, prefix string) map[string]any {
	result := make(map[string]any)
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch value := value.(type) {
		case map[string]any:
			subResult := FlattenJSON(value, fullKey)
			maps.Copy(result, subResult)
		case []any:
			for i, v := range value {
				subResult := FlattenJSON(map[string]any{fmt.Sprintf("%d", i): v}, fullKey)
				maps.Copy(result, subResult)
			}
		case int, int32, int64:
			v := reflect.ValueOf(value)
			result[fullKey] = float64(v.Int())
		case float32:
			result[fullKey] = float64(value)
		case float64, bool:
			result[fullKey] = value
		default:
			result[fullKey] = fmt.Sprintf("%v", value)
		}
	}
	return result
}
