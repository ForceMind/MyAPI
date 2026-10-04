package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCodexModelsDistinguishesEmptyFromMissingEvidence(t *testing.T) {
	for _, test := range []struct {
		name, body string
		want       []string
		invalid    bool
	}{
		{name: "oversized catalogue", body: strings.Repeat(" ", (4<<20)+1), invalid: true},
		{name: "empty catalogue", body: `{"models":[]}`, want: []string{}},
		{name: "missing catalogue", body: `{}`, invalid: true},
		{name: "null catalogue", body: `{"models":null}`, invalid: true},
		{name: "invalid catalogue", body: `{"models":"wrong"}`, invalid: true},
		{name: "missing slug", body: `{"models":[{}]}`, invalid: true},
		{name: "null entry", body: `{"models":[null]}`, invalid: true},
		{name: "empty slug", body: `{"models":[{"slug":""}]}`, invalid: true},
		{name: "null slug", body: `{"models":[{"slug":null}]}`, invalid: true},
		{name: "whitespace slug", body: `{"models":[{"slug":"   "}]}`, invalid: true},
		{name: "mixed valid and missing slug", body: `{"models":[{"slug":"partial-must-not-persist"},{}]}`, invalid: true},
		{name: "valid unique IDs", body: `{"models":[{"slug":"codex-model"},{"slug":"codex-model"},{"slug":"other-model"}]}`, want: []string{"codex-model", "other-model"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/backend-api/codex/models", r.URL.Path)
				assert.Equal(t, "fixture-version", r.URL.Query().Get("client_version"))
				_, _ = fmt.Fprint(w, test.body)
			}))
			defer upstream.Close()
			status, ids, err := FetchCodexModels(context.Background(), upstream.Client(), upstream.URL, &CodexOAuthKey{AccessToken: "synthetic-token", AccountID: "synthetic-account"}, "fixture-version")
			require.Equal(t, http.StatusOK, status)
			if test.invalid {
				require.Error(t, err)
				assert.Nil(t, ids)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, ids)
		})
	}
}
