package openai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeLifecycleRequiresTerminalForEachConcurrentIdentity(t *testing.T) {
	var state realtimeResponseLifecycle
	for _, id := range []string{"a", "b"} {
		require.NoError(t, state.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreate}))
		require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: id, Status: "in_progress"}}))
	}
	assert.True(t, state.needsReview())
	require.NoError(t, state.observeClient(&dto.RealtimeEvent{Type: "response.cancel"}))
	assert.True(t, state.needsReview(), "cancellation request is not usage evidence")
	for index, id := range []string{"b", "a"} {
		require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: id, Status: "cancelled"}}))
		assert.Equal(t, index == 0, state.needsReview())
	}
	assert.Empty(t, state.active, "completed history is not retained without bound")
	require.Error(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: "a"}}))
	assert.True(t, state.needsReview(), "duplicate terminal cannot create a second settled response")
}

func TestRealtimeLifecycleRejectsUnmatchedOrNonterminalCompletion(t *testing.T) {
	for _, event := range []*dto.RealtimeEvent{
		{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: "foreign"}},
		{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: "active", Status: "in_progress"}},
		{Type: dto.RealtimeEventTypeResponseDone},
		{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: "active"}},
	} {
		var state realtimeResponseLifecycle
		require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: "active"}}))
		require.Error(t, state.observeTarget(event))
		assert.True(t, state.needsReview())
		assert.Len(t, state.active, 1)
	}
}

func TestRealtimeLifecycleDoesNotAssignAutomaticRepliesToUnconfirmedManualRequests(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			var state realtimeResponseLifecycle
			require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeSessionCreated, Session: &dto.RealtimeSession{AutomaticResponseDisabled: disabled}}))
			// A client-side declaration must never replace the server's effective state.
			require.NoError(t, state.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeSessionUpdate, Session: &dto.RealtimeSession{AutomaticResponseDisabled: true}}))
			require.NoError(t, state.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventInputAudioBufferAppend, Audio: "AAAAAA=="}))
			require.NoError(t, state.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreate}))
			require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: "response"}}))
			require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: "response", Status: "completed"}}))
			assert.Equal(t, !disabled, state.needsReview())
			if !disabled {
				assert.Equal(t, 1, state.pending)
				require.NoError(t, state.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeSessionUpdated, Session: &dto.RealtimeSession{AutomaticResponseDisabled: true}}))
				assert.True(t, state.needsReview(), "later mode change cannot resolve earlier ambiguous work")
			}
		})
	}
	var automatic realtimeResponseLifecycle
	require.NoError(t, automatic.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventInputAudioBufferAppend, Audio: "AAAAAA=="}))
	require.NoError(t, automatic.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: "vad"}}))
	require.NoError(t, automatic.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseDone, Response: &dto.RealtimeResponse{ID: "vad"}}))
	assert.False(t, automatic.needsReview(), "pure automatic completed responses remain usable")
}

func TestRealtimeLifecycleBoundsOnlyOutstandingWork(t *testing.T) {
	var pending, active realtimeResponseLifecycle
	for i := range maxRealtimePendingResponses {
		require.NoError(t, pending.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreate}))
		require.NoError(t, active.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: fmt.Sprint(i)}}))
	}
	require.Error(t, pending.observeClient(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreate}))
	assert.Equal(t, maxRealtimePendingResponses, pending.pending)
	require.Error(t, active.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: "overflow"}}))
	assert.Len(t, active.active, maxRealtimePendingResponses)
	var oversized realtimeResponseLifecycle
	require.Error(t, oversized.observeTarget(&dto.RealtimeEvent{Type: dto.RealtimeEventTypeResponseCreated, Response: &dto.RealtimeResponse{ID: strings.Repeat("x", maxRealtimeResponseIDBytes+1)}}))
	assert.Empty(t, oversized.active)
	assert.True(t, oversized.needsReview())
}
