package config

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

var durationType = reflect.TypeFor[Duration]()

// applyEnv overrides fields from environment variables named PREFIX_<YAML_PATH>, e.g. FLUI_SERVER_TLS_CERT_FILE.
// Lists are comma-separated.
func applyEnv(cfg any, prefix string, environ []string) error {
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, prefix+"_") {
			env[k] = v
		}
	}
	if len(env) == 0 {
		return nil
	}
	return walkEnv(reflect.ValueOf(cfg).Elem(), prefix, env)
}

func walkEnv(v reflect.Value, name string, env map[string]string) error {
	t := v.Type()
	for i := range t.NumField() {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		key := name + "_" + strings.ToUpper(tag)
		f := v.Field(i)
		if f.Kind() == reflect.Struct && f.Type() != durationType {
			if err := walkEnv(f, key, env); err != nil {
				return err
			}
			continue
		}
		raw, ok := env[key]
		if !ok {
			continue
		}
		if err := setField(f, raw); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}

func setField(f reflect.Value, raw string) error {
	if f.Type() == durationType {
		d, err := ParseDuration(raw)
		if err != nil {
			return err
		}
		f.Set(reflect.ValueOf(d))
		return nil
	}
	switch f.Kind() {
	case reflect.String:
		f.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid boolean %q", raw)
		}
		f.SetBool(b)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid integer %q", raw)
		}
		f.SetInt(n)
	case reflect.Slice:
		var items []string
		for s := range strings.SplitSeq(raw, ",") {
			if s = strings.TrimSpace(s); s != "" {
				items = append(items, s)
			}
		}
		f.Set(reflect.ValueOf(items))
	default:
		return fmt.Errorf("unsupported type %s", f.Type())
	}
	return nil
}
