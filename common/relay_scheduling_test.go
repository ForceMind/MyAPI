package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRelaySchedulingLimitsAreOptInAndBounded(t *testing.T) {
	for _, test := range []struct {
		value string
		want  int
	}{{"", 0}, {"-1", 0}, {"bad", 0}, {"301", 0}, {"300", 300}, {"5", 5}} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("RELAY_FAILURE_COOLDOWN_SECONDS", test.value)
			assert.Equal(t, test.want, boundedRelaySchedulingEnv("RELAY_FAILURE_COOLDOWN_SECONDS", 300))
		})
	}
}
