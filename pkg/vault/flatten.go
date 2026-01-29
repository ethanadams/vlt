package vault

// Flatten converts a nested map structure into a flat map with dot-notation keys.
// For example: {"admin": {"oauth2": {"clientID": "x"}}} becomes {"admin.oauth2.clientID": "x"}
func Flatten(data map[string]any) map[string]any {
	result := make(map[string]any)
	flattenRecursive(data, "", result)
	return result
}

func flattenRecursive(data map[string]any, prefix string, result map[string]any) {
	for key, value := range data {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]any:
			flattenRecursive(v, fullKey, result)
		default:
			result[fullKey] = value
		}
	}
}
