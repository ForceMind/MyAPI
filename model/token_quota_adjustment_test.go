package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ForceMind/MyAPI/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newTokenQuotaAdjustmentCommandForTest(t *testing.T, token *Token, expectedVersion int64, metadataFingerprint string) TokenQuotaAdjustmentCommand {
	t.Helper()
	requestID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s:%d:%d:%d:%s", t.Name(), token.Id, token.RemainQuota, expectedVersion, token.Name))).String()
	payload, err := common.Marshal(struct {
		RequestID string `json:"request_id"`
		UserID    int    `json:"user_id"`
		TokenID   int    `json:"token_id"`
		Target    int64  `json:"target"`
		Version   int64  `json:"version"`
		Metadata  string `json:"metadata_fingerprint"`
		Name      string `json:"name"`
	}{requestID, token.UserId, token.Id, token.RemainQuota, expectedVersion, metadataFingerprint, token.Name})
	require.NoError(t, err)
	digest := sha256.Sum256(payload)
	return TokenQuotaAdjustmentCommand{
		UserID: token.UserId, RequestID: requestID, FingerprintVersion: TokenQuotaAdjustmentFingerprintVersion,
		RequestFingerprint:   fmt.Sprintf("%x", digest[:]),
		ExpectedQuotaVersion: expectedVersion, ExpectedMetadataFingerprint: metadataFingerprint,
	}
}

func TestTokenQuotaAdjustmentRejectsNoncanonicalRequestIdentity(t *testing.T) {
	validFingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("fingerprint")))
	base := TokenQuotaAdjustmentCommand{
		UserID: 1, RequestID: "de305d54-75b4-431b-adb2-eb6b9e546014", FingerprintVersion: TokenQuotaAdjustmentFingerprintVersion,
		RequestFingerprint:   validFingerprint,
		ExpectedQuotaVersion: 1, ExpectedMetadataFingerprint: validFingerprint,
	}
	for _, tc := range []struct {
		name   string
		mutate func(*TokenQuotaAdjustmentCommand)
	}{
		{"uppercase uuid", func(command *TokenQuotaAdjustmentCommand) { command.RequestID = "DE305D54-75B4-431B-ADB2-EB6B9E546014" }},
		{"uuid whitespace", func(command *TokenQuotaAdjustmentCommand) { command.RequestID += " " }},
		{"nil uuid", func(command *TokenQuotaAdjustmentCommand) { command.RequestID = uuid.Nil.String() }},
		{"unsupported fingerprint version", func(command *TokenQuotaAdjustmentCommand) { command.FingerprintVersion++ }},
		{"short request fingerprint", func(command *TokenQuotaAdjustmentCommand) { command.RequestFingerprint = "abcd" }},
		{"uppercase metadata fingerprint", func(command *TokenQuotaAdjustmentCommand) {
			command.ExpectedMetadataFingerprint = "ABCDEF" + validFingerprint[6:]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := base
			tc.mutate(&command)
			_, _, err := normalizeTokenQuotaAdjustmentCommand(command)
			require.ErrorIs(t, err, ErrTokenQuotaAdjustmentInvalidRequest)
		})
	}
}

func TestTokenQuotaAdjustmentResultPreservesWideAmountsAndExactVersions(t *testing.T) {
	adjustment := TokenQuotaAdjustment{
		RequestID: "de305d54-75b4-431b-adb2-eb6b9e546014", TokenID: 41,
		RemainQuotaBefore: 1<<32 + 17, RemainQuotaAfter: 1<<32 + 19, UsedQuotaSnapshot: 1<<32 + 23,
		QuotaVersionBefore: 1<<53 + 1, QuotaVersionAfter: 1<<53 + 2,
	}
	result := adjustment.Result()
	require.Equal(t, &TokenQuotaAdjustmentResult{
		RequestID: "de305d54-75b4-431b-adb2-eb6b9e546014", TokenID: 41,
		RemainQuotaBefore: 1<<32 + 17, RemainQuotaAfter: 1<<32 + 19, UsedQuotaSnapshot: 1<<32 + 23,
		QuotaVersionBefore: "9007199254740993", QuotaVersionAfter: "9007199254740994",
	}, result)
	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	require.Equal(t, `{"request_id":"de305d54-75b4-431b-adb2-eb6b9e546014","token_id":41,"remain_quota_before":4294967313,"remain_quota_after":4294967315,"used_quota_snapshot":4294967319,"quota_version_before":"9007199254740993","quota_version_after":"9007199254740994"}`, string(encoded))
}

// runTokenQuotaAdjustmentContract is shared by the disposable SQLite,
// MySQL 5.7 and PostgreSQL 9.6 migration fixtures. The caller must migrate the
// production TokenQuotaAdjustment model before invoking it.
func runTokenQuotaAdjustmentContract(t *testing.T, db *gorm.DB) {
	t.Helper()
	oldDB, oldLogDB := DB, LOG_DB
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	DB, LOG_DB = db, db
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch
	})

	setQuotaWriterStateForTest(t, db, QuotaWriterModeLegacy, 9401)
	const (
		userID = 940001
		wide   = int64(1<<32 + 117)
	)
	base := Token{
		UserId: userID, Key: "token-quota-adjustment-contract", Name: "opened", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: wide, UsedQuota: wide + 9, QuotaVersion: 1, Group: "default", AccessProfileID: "standard",
	}
	require.NoError(t, db.Create(&base).Error)
	openedFingerprint, err := base.QuotaMetadataFingerprint()
	require.NoError(t, err)

	first := base
	first.Name = "committed"
	first.RemainQuota = 700
	command := newTokenQuotaAdjustmentCommandForTest(t, &first, base.QuotaVersion, openedFingerprint)
	result, replayed, err := first.UpdateWithQuota(command)
	require.NoError(t, err)
	require.False(t, replayed)
	require.Equal(t, &TokenQuotaAdjustmentResult{
		RequestID: command.RequestID, TokenID: base.Id, RemainQuotaBefore: wide, RemainQuotaAfter: 700,
		UsedQuotaSnapshot: wide + 9, QuotaVersionBefore: "1", QuotaVersionAfter: "2",
	}, result)

	// A later writer changes both accounting and metadata. Replay returns the
	// original receipt together with the current row and writes neither state.
	require.NoError(t, db.Model(&Token{}).Where("id = ?", base.Id).Updates(map[string]any{
		"name": "later-metadata", "remain_quota": int64(650), "used_quota": wide + 59, "quota_version": int64(3),
	}).Error)
	retry := first
	replayedResult, replayed, err := retry.UpdateWithQuota(command)
	require.NoError(t, err)
	require.True(t, replayed)
	require.Equal(t, result, replayedResult)
	require.Equal(t, "later-metadata", retry.Name)
	require.Equal(t, int64(650), retry.RemainQuota)
	require.Equal(t, wide+59, retry.UsedQuota)
	require.Equal(t, int64(3), retry.QuotaVersion)
	var receiptCount int64
	require.NoError(t, db.Model(&TokenQuotaAdjustment{}).Where("user_id = ?", userID).Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)
	var storedAdjustment TokenQuotaAdjustment
	require.NoError(t, db.Where("request_id = ?", command.RequestID).First(&storedAdjustment).Error)
	require.Equal(t, TokenQuotaAdjustmentFingerprintVersion, storedAdjustment.FingerprintVersion)

	different := command
	different.RequestFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte("different payload")))
	_, _, err = retry.UpdateWithQuota(different)
	require.ErrorIs(t, err, ErrTokenQuotaAdjustmentConflict)

	other := Token{
		UserId: userID, Key: "token-quota-adjustment-contract-other", Name: "other", Status: common.TokenStatusEnabled,
		ExpiredTime: -1, RemainQuota: 10, QuotaVersion: 1, Group: "default", AccessProfileID: "standard",
	}
	require.NoError(t, db.Create(&other).Error)
	crossToken := command
	_, _, err = other.UpdateWithQuota(crossToken)
	require.ErrorIs(t, err, ErrTokenQuotaAdjustmentConflict)

	rollbackOpened, err := GetTokenById(other.Id)
	require.NoError(t, err)
	rollbackFingerprint, err := rollbackOpened.QuotaMetadataFingerprint()
	require.NoError(t, err)
	rollbackOpened.RemainQuota = 9
	rollbackCommand := newTokenQuotaAdjustmentCommandForTest(t, rollbackOpened, rollbackOpened.QuotaVersion, rollbackFingerprint)
	rollbackErr := errors.New("synthetic receipt failure")
	callbackName := "token-quota-adjustment-contract-receipt-failure"
	rollbackCallbackRegistered := true
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == (TokenQuotaAdjustment{}).TableName() {
			tx.AddError(rollbackErr)
		}
	}))
	t.Cleanup(func() {
		if rollbackCallbackRegistered {
			_ = db.Callback().Create().Remove(callbackName)
		}
	})
	_, _, err = rollbackOpened.UpdateWithQuota(rollbackCommand)
	require.ErrorIs(t, err, rollbackErr)
	require.NoError(t, db.Callback().Create().Remove(callbackName))
	rollbackCallbackRegistered = false
	var afterRollback Token
	require.NoError(t, db.First(&afterRollback, other.Id).Error)
	require.Equal(t, int64(10), afterRollback.RemainQuota)
	require.Equal(t, int64(1), afterRollback.QuotaVersion)
	require.NoError(t, db.Model(&TokenQuotaAdjustment{}).Where("request_id = ?", rollbackCommand.RequestID).Count(&receiptCount).Error)
	require.Zero(t, receiptCount)

	concurrentOpened := afterRollback
	concurrentOpened.RemainQuota = 8
	concurrentCommand := newTokenQuotaAdjustmentCommandForTest(t, &concurrentOpened, afterRollback.QuotaVersion, rollbackFingerprint)
	type outcome struct {
		result   *TokenQuotaAdjustmentResult
		replayed bool
		err      error
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			candidate := concurrentOpened
			<-start
			got, wasReplay, updateErr := candidate.UpdateWithQuota(concurrentCommand)
			outcomes <- outcome{got, wasReplay, updateErr}
		}()
	}
	close(start)
	workers.Wait()
	close(outcomes)
	seenFresh, seenReplay := 0, 0
	for got := range outcomes {
		require.NoError(t, got.err)
		require.NotNil(t, got.result)
		if got.replayed {
			seenReplay++
		} else {
			seenFresh++
		}
	}
	require.Equal(t, 1, seenFresh)
	require.Equal(t, 1, seenReplay)
	require.NoError(t, db.Model(&TokenQuotaAdjustment{}).Where("request_id = ?", concurrentCommand.RequestID).Count(&receiptCount).Error)
	require.Equal(t, int64(1), receiptCount)
	var concurrentStored Token
	require.NoError(t, db.First(&concurrentStored, other.Id).Error)
	require.Equal(t, int64(8), concurrentStored.RemainQuota)
	require.Equal(t, int64(2), concurrentStored.QuotaVersion)

	if db.Dialector.Name() != "sqlite" {
		left := Token{
			UserId: userID, Key: "token-quota-adjustment-native-left", Name: "left-before", Status: common.TokenStatusEnabled,
			ExpiredTime: -1, RemainQuota: 31, QuotaVersion: 1, Group: "default", AccessProfileID: "standard",
		}
		right := Token{
			UserId: userID, Key: "token-quota-adjustment-native-right", Name: "right-before", Status: common.TokenStatusEnabled,
			ExpiredTime: -1, RemainQuota: 41, QuotaVersion: 1, Group: "default", AccessProfileID: "standard",
		}
		require.NoError(t, db.Create(&left).Error)
		require.NoError(t, db.Create(&right).Error)
		leftFingerprint, err := left.QuotaMetadataFingerprint()
		require.NoError(t, err)
		rightFingerprint, err := right.QuotaMetadataFingerprint()
		require.NoError(t, err)
		leftCandidate, rightCandidate := left, right
		leftCandidate.Name, leftCandidate.RemainQuota = "left-after", 30
		rightCandidate.Name, rightCandidate.RemainQuota = "right-after", 40
		leftCommand := newTokenQuotaAdjustmentCommandForTest(t, &leftCandidate, 1, leftFingerprint)
		rightCommand := newTokenQuotaAdjustmentCommandForTest(t, &rightCandidate, 1, rightFingerprint)
		sharedRequestID := "c56a4180-65aa-42ec-a945-5fd21dec0538"
		leftCommand.RequestID, rightCommand.RequestID = sharedRequestID, sharedRequestID
		leftCommand.RequestFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", sharedRequestID, left.Id, leftCandidate.RemainQuota, leftCandidate.Name))))
		rightCommand.RequestFingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d:%s", sharedRequestID, right.Id, rightCandidate.RemainQuota, rightCandidate.Name))))

		arrived := make(chan struct{}, 2)
		release := make(chan struct{})
		callbackName := "token-quota-adjustment-native-unique-race"
		callbackRegistered := true
		require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
			adjustment, ok := tx.Statement.Dest.(*TokenQuotaAdjustment)
			if !ok || adjustment.RequestID != sharedRequestID {
				return
			}
			arrived <- struct{}{}
			<-release
		}))
		t.Cleanup(func() {
			if callbackRegistered {
				_ = db.Callback().Create().Remove(callbackName)
			}
		})
		type nativeOutcome struct {
			tokenID  int
			replayed bool
			err      error
		}
		nativeOutcomes := make(chan nativeOutcome, 2)
		var nativeWorkers sync.WaitGroup
		for _, work := range []struct {
			candidate Token
			command   TokenQuotaAdjustmentCommand
		}{{leftCandidate, leftCommand}, {rightCandidate, rightCommand}} {
			nativeWorkers.Add(1)
			go func(work struct {
				candidate Token
				command   TokenQuotaAdjustmentCommand
			}) {
				defer nativeWorkers.Done()
				_, wasReplay, updateErr := work.candidate.UpdateWithQuota(work.command)
				nativeOutcomes <- nativeOutcome{work.candidate.Id, wasReplay, updateErr}
			}(work)
		}
		raceContext, cancelRace := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelRace()
		for arrivalCount := 0; arrivalCount < 2; {
			select {
			case <-arrived:
				arrivalCount++
			case early := <-nativeOutcomes:
				close(release)
				nativeWorkers.Wait()
				require.FailNow(t, "native unique-race worker exited before the receipt barrier", "token=%d replayed=%v err=%v", early.tokenID, early.replayed, early.err)
			case <-raceContext.Done():
				close(release)
				nativeWorkers.Wait()
				require.FailNow(t, "native unique-race receipt barrier was not reached twice", "%v", raceContext.Err())
			}
		}
		cancelRace()
		close(release)
		nativeWorkers.Wait()
		close(nativeOutcomes)
		require.NoError(t, db.Callback().Create().Remove(callbackName))
		callbackRegistered = false

		winnerID, loserID := 0, 0
		for outcome := range nativeOutcomes {
			if outcome.err == nil {
				require.False(t, outcome.replayed)
				require.Zero(t, winnerID)
				winnerID = outcome.tokenID
				continue
			}
			require.ErrorIs(t, outcome.err, ErrTokenQuotaAdjustmentConflict)
			require.False(t, outcome.replayed)
			require.Zero(t, loserID)
			loserID = outcome.tokenID
		}
		require.NotZero(t, winnerID)
		require.NotZero(t, loserID)
		require.NoError(t, db.Model(&TokenQuotaAdjustment{}).Where("request_id = ?", sharedRequestID).Count(&receiptCount).Error)
		require.Equal(t, int64(1), receiptCount)
		var storedLeft, storedRight Token
		require.NoError(t, db.First(&storedLeft, left.Id).Error)
		require.NoError(t, db.First(&storedRight, right.Id).Error)
		for _, stored := range []Token{storedLeft, storedRight} {
			if stored.Id == winnerID {
				require.Equal(t, int64(2), stored.QuotaVersion)
				if stored.Id == left.Id {
					require.Equal(t, "left-after", stored.Name)
					require.Equal(t, int64(30), stored.RemainQuota)
				} else {
					require.Equal(t, "right-after", stored.Name)
					require.Equal(t, int64(40), stored.RemainQuota)
				}
				continue
			}
			require.Equal(t, loserID, stored.Id)
			require.Equal(t, int64(1), stored.QuotaVersion)
			if stored.Id == left.Id {
				require.Equal(t, "left-before", stored.Name)
				require.Equal(t, int64(31), stored.RemainQuota)
			} else {
				require.Equal(t, "right-before", stored.Name)
				require.Equal(t, int64(41), stored.RemainQuota)
			}
		}
	}
}

func TestTokenQuotaAdjustmentContractSQLite(t *testing.T) {
	setupUserUpdateTestState(t)
	require.NoError(t, DB.AutoMigrate(&TokenQuotaAdjustment{}))
	runTokenQuotaAdjustmentContract(t, DB)
}
