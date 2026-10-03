package openai

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/ForceMind/MyAPI/common"
	"github.com/ForceMind/MyAPI/logger"
	relaycommon "github.com/ForceMind/MyAPI/relay/common"
	"github.com/ForceMind/MyAPI/relay/helper"
	"github.com/ForceMind/MyAPI/relaykit/dto"
	"github.com/ForceMind/MyAPI/relaykit/types"
	"github.com/ForceMind/MyAPI/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn, targetConn := info.ClientWs, info.TargetWs
	requestContext := context.Background()
	if c.Request != nil {
		requestContext = c.Request.Context()
	}
	readerContext, stopReaders := context.WithCancel(requestContext)
	defer stopReaders()

	clientClosed, targetClosed := make(chan struct{}), make(chan struct{})
	errChan := make(chan error, 2)
	var stateMu sync.Mutex
	var readers sync.WaitGroup
	localUsage, sumUsage := &dto.RealtimeUsage{}, &dto.RealtimeUsage{}
	var lifecycle realtimeResponseLifecycle

	handleClient := func(event *dto.RealtimeEvent) error {
		stateMu.Lock()
		defer stateMu.Unlock()
		if err := lifecycle.observeClient(event); err != nil {
			return err
		}
		if event.Type == dto.RealtimeEventTypeSessionUpdate && event.Session != nil && event.Session.Tools != nil {
			info.RealtimeTools = event.Session.Tools
		}
		textToken, audioToken, err := service.CountTokenRealtime(info, *event, info.UpstreamModelName)
		if err != nil {
			return fmt.Errorf("error counting text token: %w", err)
		}
		localUsage.TotalTokens += textToken + audioToken
		localUsage.InputTokens += textToken + audioToken
		localUsage.InputTokenDetails.TextTokens += textToken
		localUsage.InputTokenDetails.AudioTokens += audioToken
		return nil
	}

	handleTarget := func(event *dto.RealtimeEvent) error {
		stateMu.Lock()
		defer stateMu.Unlock()
		info.SetFirstResponseTime()
		if err := lifecycle.observeTarget(event); err != nil {
			info.RealtimeUsageUnverified = true
			sumUsage.UsageIncomplete = true
			return err
		}
		switch event.Type {
		case dto.RealtimeEventTypeResponseDone:
			if event.Response == nil {
				info.RealtimeUsageUnverified = true
				sumUsage.UsageIncomplete = true
				return fmt.Errorf("response.done is missing response")
			}
			if event.Response.Usage != nil {
				// Reported usage supersedes the local estimate even if the
				// reservation fails. Its counts already enter sumUsage;
				// neither bucket may be consumed again during shutdown.
				localUsage = &dto.RealtimeUsage{}
				if err := preConsumeUsage(c, info, event.Response.Usage, sumUsage, true); err != nil {
					return fmt.Errorf("error consume usage: %w", err)
				}
				return nil
			}
			textToken, audioToken, err := service.CountTokenRealtime(info, *event, info.UpstreamModelName)
			if err != nil {
				return fmt.Errorf("error counting text token: %w", err)
			}
			localUsage.TotalTokens += textToken + audioToken
			localUsage.InputTokens += textToken + audioToken
			localUsage.InputTokenDetails.TextTokens += textToken
			localUsage.InputTokenDetails.AudioTokens += audioToken
			info.IsFirstRequest = false
			completed := localUsage
			localUsage = &dto.RealtimeUsage{}
			if err := preConsumeUsage(c, info, completed, sumUsage); err != nil {
				return fmt.Errorf("error consume usage: %w", err)
			}
		case dto.RealtimeEventTypeSessionUpdated, dto.RealtimeEventTypeSessionCreated:
			if event.Session != nil {
				info.InputAudioFormat = common.GetStringIfEmpty(event.Session.InputAudioFormat, info.InputAudioFormat)
				info.OutputAudioFormat = common.GetStringIfEmpty(event.Session.OutputAudioFormat, info.OutputAudioFormat)
			}
		default:
			textToken, audioToken, err := service.CountTokenRealtime(info, *event, info.UpstreamModelName)
			if err != nil {
				return fmt.Errorf("error counting text token: %w", err)
			}
			localUsage.TotalTokens += textToken + audioToken
			localUsage.OutputTokens += textToken + audioToken
			localUsage.OutputTokenDetails.TextTokens += textToken
			localUsage.OutputTokenDetails.AudioTokens += audioToken
		}
		return nil
	}

	readSocket := func(source, destination *websocket.Conn, label string, closed chan struct{}, handle func(*dto.RealtimeEvent) error) {
		defer readers.Done()
		defer close(closed)
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in %s reader: %v", label, r)
			}
		}()
		for {
			select {
			case <-readerContext.Done():
				return
			default:
			}
			_, message, err := source.ReadMessage()
			if err != nil {
				if readerContext.Err() == nil && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- fmt.Errorf("error reading from %s: %w", label, err)
				}
				return
			}
			event := &dto.RealtimeEvent{}
			if err := common.Unmarshal(message, event); err != nil {
				if label == "target" {
					stateMu.Lock()
					info.RealtimeUsageUnverified = true
					sumUsage.UsageIncomplete = true
					stateMu.Unlock()
				}
				errChan <- fmt.Errorf("error unmarshalling message: %w", err)
				return
			}
			if err := handle(event); err != nil {
				errChan <- err
				return
			}
			if err := helper.WssString(c, destination, string(message)); err != nil {
				if readerContext.Err() == nil {
					errChan <- fmt.Errorf("error forwarding from %s: %w", label, err)
				}
				return
			}
		}
	}

	readers.Add(2)
	gopool.Go(func() { readSocket(clientConn, targetConn, "client", clientClosed, handleClient) })
	gopool.Go(func() { readSocket(targetConn, clientConn, "target", targetClosed, handleTarget) })

	var terminalErr error
	select {
	case <-clientClosed:
	case <-targetClosed:
	case terminalErr = <-errChan:
	case <-readerContext.Done():
	}

	// Close unblocks both network reads/writes. No reader may mutate usage or
	// relay metadata after the snapshot is returned for final settlement.
	stopReaders()
	_ = clientConn.Close()
	_ = targetConn.Close()
	readers.Wait()
	if terminalErr == nil {
		select {
		case terminalErr = <-errChan:
		default:
		}
	}
	if terminalErr != nil {
		logger.LogError(c, "realtime error: "+terminalErr.Error())
	}
	if lifecycle.needsReview() {
		info.RealtimeUsageUnverified = true
		sumUsage.UsageIncomplete = true
	}
	if localUsage.TotalTokens != 0 {
		if err := preConsumeUsage(c, info, localUsage, sumUsage); err != nil {
			logger.LogError(c, "realtime tail reservation failed: "+err.Error())
		}
	}
	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage, reported ...bool) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	if usage.UsageIncomplete {
		info.RealtimeUsageUnverified = true
		totalUsage.UsageIncomplete = true
		return fmt.Errorf("realtime usage lacks complete reported counts")
	}

	// Validate the whole addition before publishing it. A rejected segment must
	// not wrap counts or partially change a previously known aggregate.
	next := *totalUsage
	next.InputTokenDetails = dto.CloneInputTokenDetails(totalUsage.InputTokenDetails)
	values := []struct {
		destination *int
		delta       int
	}{
		{&next.TotalTokens, usage.TotalTokens}, {&next.InputTokens, usage.InputTokens}, {&next.OutputTokens, usage.OutputTokens},
		{&next.InputTokenDetails.CachedTokens, usage.InputTokenDetails.CachedTokens},
		{&next.InputTokenDetails.TextTokens, usage.InputTokenDetails.TextTokens},
		{&next.InputTokenDetails.AudioTokens, usage.InputTokenDetails.AudioTokens},
		{&next.InputTokenDetails.ImageTokens, usage.InputTokenDetails.ImageTokens},
		{&next.OutputTokenDetails.TextTokens, usage.OutputTokenDetails.TextTokens},
		{&next.OutputTokenDetails.AudioTokens, usage.OutputTokenDetails.AudioTokens},
		{&next.OutputTokenDetails.ImageTokens, usage.OutputTokenDetails.ImageTokens},
	}
	if cached := usage.InputTokenDetails.CachedTokensDetails; cached != nil {
		if next.InputTokenDetails.CachedTokensDetails == nil {
			next.InputTokenDetails.CachedTokensDetails = &dto.CachedTokenDetails{}
		}
		totalCached := next.InputTokenDetails.CachedTokensDetails
		for _, part := range []struct {
			source      *int
			destination **int
		}{
			{cached.TextTokens, &totalCached.TextTokens}, {cached.AudioTokens, &totalCached.AudioTokens}, {cached.ImageTokens, &totalCached.ImageTokens},
		} {
			if part.source == nil {
				continue
			}
			if *part.destination == nil {
				*part.destination = common.GetPointer(0)
			}
			values = append(values, struct {
				destination *int
				delta       int
			}{*part.destination, *part.source})
		}
	}
	for _, value := range values {
		if *value.destination < 0 || *value.destination > math.MaxInt32 || value.delta < 0 || value.delta > math.MaxInt32-*value.destination {
			info.RealtimeUsageUnverified = true
			totalUsage.UsageIncomplete = true
			return fmt.Errorf("realtime usage accumulation exceeds supported counts")
		}
		*value.destination += value.delta
	}
	*totalUsage = next
	if len(reported) > 0 && reported[0] {
		info.RealtimeReportedUsage = true
		if err := service.RecordRealtimeTieredResponse(info, usage); err != nil {
			return err
		}
	} else {
		info.RealtimeUsageUnverified = true
		if info.TieredBillingSnapshot != nil {
			if info.RealtimeTieredPricing == nil {
				info.RealtimeTieredPricing = &relaycommon.RealtimeTieredPricing{}
			}
			info.RealtimeTieredPricing.Incomplete = true
		}
	}
	// Preserve known counts for final settlement even if extending the
	// reservation fails. The reader owns clearing the completed bucket.
	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}
