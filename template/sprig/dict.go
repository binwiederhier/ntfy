package sprig

import "fmt"

// get retrieves a value from a map by its key.
// If the key exists, returns the corresponding value.
// If the key doesn't exist, returns an empty string.
//
// Parameters:
//   - d: The map to retrieve the value from
//   - key: The key to look up
//
// Returns:
//   - any: The value associated with the key, or an empty string if not found
func get(d map[string]any, key string) any {
	if val, ok := d[key]; ok {
		return val
	}
	return ""
}

// set adds or updates a key-value pair in a map.
// Modifies the map in place and returns the modified map.
//
// Parameters:
//   - d: The map to modify
//   - key: The key to set
//   - value: The value to associate with the key
//
// Returns:
//   - map[string]any: The modified map (same instance as the input map)
func set(d map[string]any, key string, value any) map[string]any {
	d[key] = value
	return d
}

// unset removes a key-value pair from a map.
// If the key doesn't exist, the map remains unchanged.
// Modifies the map in place and returns the modified map.
//
// Parameters:
//   - d: The map to modify
//   - key: The key to remove
//
// Returns:
//   - map[string]any: The modified map (same instance as the input map)
func unset(d map[string]any, key string) map[string]any {
	delete(d, key)
	return d
}

// hasKey checks if a key exists in a map.
//
// Parameters:
//   - d: The map to check
//   - key: The key to look for
//
// Returns:
//   - bool: True if the key exists in the map, false otherwise
func hasKey(d map[string]any, key string) bool {
	_, ok := d[key]
	return ok
}

// pluck extracts values for a specific key from multiple maps.
// Only includes values from maps where the key exists.
//
// Parameters:
//   - key: The key to extract values for
//   - d: A variadic list of maps to extract values from
//
// Returns:
//   - []any: A slice containing all values associated with the key across all maps
func pluck(key string, d ...map[string]any) []any {
	var res []any
	for _, dict := range d {
		if val, ok := dict[key]; ok {
			res = append(res, val)
		}
	}
	return res
}

// keys collects all keys from one or more maps.
// The returned slice may contain duplicate keys if multiple maps contain the same key.
//
// Parameters:
//   - dicts: A variadic list of maps to collect keys from
//
// Returns:
//   - []string: A slice containing all keys from all provided maps
func keys(dicts ...map[string]any) []string {
	// Passing the same dict many times would otherwise multiply it in a single call
	total := 0
	for _, dict := range dicts {
		total += len(dict)
	}
	if total > sliceSizeLimit {
		panic(fmt.Sprintf("keys result size %d exceeds limit of %d", total, sliceSizeLimit))
	}
	k := make([]string, 0, total)
	for _, dict := range dicts {
		for key := range dict {
			k = append(k, key)
		}
	}
	return k
}

// pick creates a new map containing only the specified keys from the original map.
// If a key doesn't exist in the original map, it won't be included in the result.
//
// Parameters:
//   - dict: The source map
//   - keys: A variadic list of keys to include in the result
//
// Returns:
//   - map[string]any: A new map containing only the specified keys and their values
func pick(dict map[string]any, keys ...string) map[string]any {
	res := map[string]any{}
	for _, k := range keys {
		if v, ok := dict[k]; ok {
			res[k] = v
		}
	}
	return res
}

// omit creates a new map excluding the specified keys from the original map.
// The original map remains unchanged.
//
// Parameters:
//   - dict: The source map
//   - keys: A variadic list of keys to exclude from the result
//
// Returns:
//   - map[string]any: A new map containing all key-value pairs except those specified
func omit(dict map[string]any, keys ...string) map[string]any {
	res := map[string]any{}
	omit := make(map[string]bool, len(keys))
	for _, k := range keys {
		omit[k] = true
	}
	for k, v := range dict {
		if _, ok := omit[k]; !ok {
			res[k] = v
		}
	}
	return res
}

// dict creates a new map from a list of key-value pairs.
// The arguments are treated as key-value pairs, where even-indexed arguments are keys
// and odd-indexed arguments are values.
// If there's an odd number of arguments, the last key will be assigned an empty string value.
//
// Parameters:
//   - v: A variadic list of alternating keys and values
//
// Returns:
//   - map[string]any: A new map containing the specified key-value pairs
func dict(v ...any) map[string]any {
	dict := map[string]any{}
	lenv := len(v)
	for i := 0; i < lenv; i += 2 {
		key := strval(v[i])
		if i+1 >= lenv {
			dict[key] = ""
			continue
		}
		dict[key] = v[i+1]
	}
	return dict
}

// values collects all values from a map into a slice.
// The order of values in the resulting slice is not guaranteed.
//
// Parameters:
//   - dict: The map to collect values from
//
// Returns:
//   - []any: A slice containing all values from the map
func values(dict map[string]any) []any {
	var values []any
	for _, value := range dict {
		values = append(values, value)
	}
	return values
}

// dig safely accesses nested values in maps using a sequence of keys.
// If any key in the path doesn't exist, it returns the default value.
// The function expects at least 3 arguments: one or more keys, a default value, and a map.
//
// Parameters:
//   - ps: A variadic list where:
//   - The first N-2 arguments are string keys forming the path
//   - The second-to-last argument is the default value to return if the path doesn't exist
//   - The last argument is the map to traverse
//
// Returns:
//   - any: The value found at the specified path, or the default value if not found
//   - error: Any error that occurred during traversal
//
// Panics:
//   - If fewer than 3 arguments are provided
func dig(ps ...any) (any, error) {
	if len(ps) < 3 {
		panic("dig needs at least three arguments")
	}
	dict := ps[len(ps)-1].(map[string]any)
	def := ps[len(ps)-2]
	ks := make([]string, len(ps)-2)
	for i := 0; i < len(ks); i++ {
		ks[i] = ps[i].(string)
	}

	return digFromDict(dict, def, ks)
}

// digFromDict is a helper function for dig that recursively traverses a map using a sequence of keys.
// If any key in the path doesn't exist, it returns the default value.
//
// Parameters:
//   - dict: The map to traverse
//   - d: The default value to return if the path doesn't exist
//   - ks: A slice of string keys forming the path to traverse
//
// Returns:
//   - any: The value found at the specified path, or the default value if not found
//   - error: Any error that occurred during traversal
func digFromDict(dict map[string]any, d any, ks []string) (any, error) {
	k, ns := ks[0], ks[1:]
	step, has := dict[k]
	if !has {
		return d, nil
	}
	if len(ns) == 0 {
		return step, nil
	}
	return digFromDict(step.(map[string]any), d, ns)
}
