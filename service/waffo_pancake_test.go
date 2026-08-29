package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaffoPancakeBuyerIdentityUsesMyAPIPrefix(t *testing.T) {
	require.Equal(t, "my-api-user-42", WaffoPancakeBuyerIdentityFromUserID(42))
}

func TestWaffoPancakeBuyerIdentityAcceptsLegacyPrefix(t *testing.T) {
	require.True(t, waffoPancakeBuyerIdentityMatchesUserID("new-api-user-42", 42))
	require.True(t, waffoPancakeBuyerIdentityMatchesUserID("my-api-user-42", 42))
	require.False(t, waffoPancakeBuyerIdentityMatchesUserID("new-api-user-43", 42))
}
