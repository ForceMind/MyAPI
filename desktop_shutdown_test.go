package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
)

func TestReadDesktopShutdown(t *testing.T) {
	tests := []struct {
		name  string
		input io.Reader
		want  bool
	}{
		{name: "exact command and EOF", input: strings.NewReader("shutdown\n"), want: true},
		{name: "parent closed its pipe", input: strings.NewReader("")},
		{name: "incomplete command", input: strings.NewReader("shutdown")},
		{name: "other command", input: strings.NewReader("restart\n")},
		{name: "another command follows", input: strings.NewReader("shutdown\nshutdown\n")},
		{name: "pipe error", input: io.MultiReader(strings.NewReader("shutdown\n"), iotest.ErrReader(errors.New("pipe error")))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, readDesktopShutdown(tt.input))
		})
	}
}
