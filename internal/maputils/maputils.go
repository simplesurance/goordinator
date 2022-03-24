package maputils

import "fmt"

// StrVal returns the value of the key as string.
// If the key does not exist an empty string is returned.
// If they key exist but has a different type an error is returned.
func StrVal(m map[string]any, key string) (string, error) {
	val, ok := m[key]
	if !ok {
		return "", nil
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("value of key %q has type %T, expected string", key, val)
	}

	return str, nil
}

// StrSliceVal returns the value of the key as []string.
// If the key does not exist nil is returned.
// If they key exist but has a different type an error is returned.
func StrSliceVal(m map[string]any, key string) ([]string, error) {
	val, ok := m[key]
	if !ok {
		return nil, nil
	}

	strSlice, ok := val.([]string)
	if !ok {
		return nil, fmt.Errorf("value of key %q has type %T, expected []string", key, val)
	}

	return strSlice, nil
}

// MapSliceVal returns the value of the key as map[string]any
// If the key does not exist an empty map is returned.
// If they key exist but has a different type an error is returned.
func MapSliceVal(m map[string]any, key string) (map[string]any, error) {
	val, ok := m[key]
	if !ok {
		return map[string]any{}, nil
	}

	iMap, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("value of key %q has type %T, expected map[string]any", key, val)
	}

	return iMap, nil
}

// ToStrMap converts the map to map[string]string.
// If a value in m is not a string an error is returned.
func ToStrMap(m map[string]any) (map[string]string, error) {
	result := make(map[string]string, len(m))

	for k, v := range m {
		strVal, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("value of key %q has type %T, expected string", k, v)
		}

		result[k] = strVal
	}

	return result, nil
}
