package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommands(t *testing.T) {
	cases := []struct {
		args     []string
		code     int
		inStdout string
		inStderr string
	}{
		{[]string{"version"}, 0, "flui-server", ""},
		{[]string{"help"}, 0, "Usage:", ""},
		{[]string{"nope"}, 2, "", "unknown command"},
		{[]string{"tls"}, 2, "", "tls fingerprint"},
		{[]string{"backup", "--config", "/does/not/exist.yaml", "x.db"}, 1, "", "invalid configuration"},
		{[]string{"serve", "--bogus"}, 2, "", "flag provided but not defined"},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		code := run(c.args, &out, &errb)
		if code != c.code || !strings.Contains(out.String(), c.inStdout) || !strings.Contains(errb.String(), c.inStderr) {
			t.Errorf("%v: code=%d stdout=%q stderr=%q", c.args, code, out.String(), errb.String())
		}
	}
}
