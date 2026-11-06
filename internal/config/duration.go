package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Duration is a time.Duration that also accepts a "d" (days) suffix, e.g. "30d".
type Duration time.Duration

const Day = 24 * time.Hour

func ParseDuration(s string) (Duration, error) {
	s = strings.TrimSpace(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseFloat(days, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return Duration(n * float64(Day)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return Duration(d), nil
}

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) String() string {
	td := time.Duration(d)
	if td >= Day && td%Day == 0 {
		return fmt.Sprintf("%dd", td/Day)
	}
	return td.String()
}

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("line %d: %w", n.Line, err)
	}
	*d = v
	return nil
}

func (d Duration) MarshalYAML() (any, error) { return d.String(), nil }
