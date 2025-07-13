package sprig

func get(d map[string]any, key string) any {
	if val, ok := d[key]; ok {
		return val
	}
	return ""
}

func set(d map[string]any, key string, value any) map[string]any {
	d[key] = value
	return d
}

func unset(d map[string]any, key string) map[string]any {
	delete(d, key)
	return d
}

func hasKey(d map[string]any, key string) bool {
	_, ok := d[key]
	return ok
}

func pluck(key string, d ...map[string]any) []any {
	var res []any
	for _, dict := range d {
		if val, ok := dict[key]; ok {
			res = append(res, val)
		}
	}
	return res
}

func keys(dicts ...map[string]any) []string {
	k := []string{}
	for _, dict := range dicts {
		for key := range dict {
			k = append(k, key)
		}
	}
	return k
}

func pick(dict map[string]any, keys ...string) map[string]any {
	res := map[string]any{}
	for _, k := range keys {
		if v, ok := dict[k]; ok {
			res[k] = v
		}
	}
	return res
}

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

func values(dict map[string]any) []any {
	var values []any
	for _, value := range dict {
		values = append(values, value)
	}
	return values
}

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
