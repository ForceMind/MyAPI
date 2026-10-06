package router

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/model"
	"github.com/ForceMind/MyAPI/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignedAccessStrictBudgetApplicationFlow(t *testing.T) {
	for _, protocol := range []string{"responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			f := setupChatBudgetFlow(t)
			token := f.token(t, model.TokenBudgetOpenAIChatModel)
			policy := fmt.Sprintf(`{"enabled":true,"public_models":[%q],"upstream_models":[%q],"channel_ids":[%d]}`, model.TokenBudgetOpenAIChatModel, model.TokenBudgetOpenAIChatModel, f.channel.Id)
			_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, policy)
			require.NoError(t, err)
			var counts, generations atomic.Int32
			client := chatBudgetFlowTLSClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "api.openai.com", r.Host)
				assert.Equal(t, "Bearer synthetic-only", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.EqualValues(t, len(body), r.ContentLength, "guard must preserve exact body framing")
				var fields map[string]any
				require.NoError(t, common.Unmarshal(body, &fields))
				assert.Equal(t, model.TokenBudgetOpenAIChatModel, fields["model"])
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/responses/input_tokens":
					require.Equal(t, "responses", protocol)
					counts.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"object":"response.input_tokens","input_tokens":100}`)
				case "/v1/responses":
					require.Equal(t, "responses", protocol)
					generations.Add(1)
					assert.Equal(t, "hello", fields["input"])
					_, _ = io.WriteString(w, `{"id":"assigned-responses","object":"response","status":"completed","model":"gpt-6.1-sol","service_tier":"default","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],"usage":{"input_tokens":100,"output_tokens":10,"total_tokens":110,"input_tokens_details":{"cached_tokens":40,"cache_write_tokens":30},"output_tokens_details":{"reasoning_tokens":4}}}`)
				case "/v1/chat/completions":
					require.Equal(t, "chat", protocol)
					generations.Add(1)
					_, _ = io.WriteString(w, chatBudgetFlowResponse(chatFlowUsage, false, false))
				default:
					t.Errorf("unexpected assigned-budget fixture path: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			})
			t.Cleanup(service.SetHttpClientForTest(client))
			path, body := "/v1/responses", `{"model":"gpt-6.1-sol","input":"hello","max_output_tokens":20,"service_tier":"default"}`
			if protocol == "chat" {
				path, body = "/v1/chat/completions", chatFlowRequest
			}
			response := f.request(http.MethodPost, path, "sk-"+token.Key, body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.EqualValues(t, 1, generations.Load())
			var row model.TokenBudgetReservation
			require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
			require.Equal(t, model.TokenBudgetSettled, row.State)
			require.NotNil(t, row.ActualInput)
			assert.EqualValues(t, 100, *row.ActualInput)
			require.NotNil(t, row.ActualOutput)
			assert.EqualValues(t, 10, *row.ActualOutput)
			require.NotNil(t, row.ActualFeeUSD)
			assert.Equal(t, "0.000239", *row.ActualFeeUSD)
			if protocol == "responses" {
				assert.EqualValues(t, 1, counts.Load())
				assert.Equal(t, model.TokenBudgetBoundOpenAIResponses, row.BoundSource)
				assert.EqualValues(t, 120, row.Reserved)
				assert.Equal(t, "0.00045", row.FeeReservedUSD)
			} else {
				assert.Zero(t, counts.Load())
				assert.Equal(t, model.TokenBudgetBoundOpenAIChat, row.BoundSource)
				assert.EqualValues(t, 1050000, row.Reserved)
				assert.Equal(t, "5.2502", row.FeeReservedUSD)
			}
			budget, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
			require.NoError(t, err)
			assert.EqualValues(t, 110, budget.Used)
			assert.Zero(t, budget.Reserved)
			assert.Equal(t, "0.000239", budget.FeeUsedUSD)
			assert.Equal(t, "0", budget.FeeReservedUSD)
			assert.Empty(t, budget.PendingRequestID)
		})
	}
}

func TestAssignedAccessStrictChatBudgetKeepsUnknownHold(t *testing.T) {
	f := setupChatBudgetFlow(t)
	token := f.token(t, model.TokenBudgetOpenAIChatModel)
	_, err := model.ConfigureAssignedAccessPolicy(context.Background(), model.DB, "token", token.Id, 0, `{"enabled":true,"public_models":["gpt-6.1-sol"],"upstream_models":["gpt-6.1-sol"],"channel_ids":null}`)
	require.NoError(t, err)
	f.setResponder(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatBudgetFlowResponse(strings.Replace(chatFlowUsage, `,"cache_write_tokens":30`, "", 1), false, false))
	})
	response := f.request(http.MethodPost, "/v1/chat/completions", "sk-"+token.Key, chatFlowRequest)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.EqualValues(t, 1, f.hits.Load())
	var row model.TokenBudgetReservation
	require.NoError(t, model.DB.Where("token_id = ?", token.Id).First(&row).Error)
	assert.Equal(t, model.TokenBudgetUnknown, row.State)
	assert.Nil(t, row.ActualInput)
	assert.Nil(t, row.ActualFeeUSD)
	budget, err := model.LookupTokenBudget(context.Background(), model.DB, token.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 1050000, budget.Reserved)
	assert.Equal(t, "5.2502", budget.FeeReservedUSD)
	assert.Zero(t, budget.Used)
	assert.Equal(t, "0", budget.FeeUsedUSD)
	assert.Equal(t, row.RequestID, budget.PendingRequestID)
}
