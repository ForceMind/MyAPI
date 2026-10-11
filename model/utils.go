package model

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ForceMind/MyAPI/common"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const (
	BatchUpdateTypeUserQuota = iota
	BatchUpdateTypeTokenQuota
	BatchUpdateTypeUsedQuota
	BatchUpdateTypeChannelUsedQuota
	BatchUpdateTypeRequestCount
	BatchUpdateTypeCount // if you add a new type, you need to add a new map and a new lock
)

var batchUpdateStores []map[int]int64
var batchUpdateLocks []sync.Mutex

func init() {
	for i := 0; i < BatchUpdateTypeCount; i++ {
		batchUpdateStores = append(batchUpdateStores, make(map[int]int64))
		batchUpdateLocks = append(batchUpdateLocks, sync.Mutex{})
	}
}

func InitBatchUpdater() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Duration(common.BatchUpdateInterval) * time.Second)
			batchUpdate()
		}
	})
}

func addNewRecord(type_ int, id int, value int64) error {
	if type_ < 0 || type_ >= BatchUpdateTypeCount || id <= 0 || value == 0 {
		return errors.New("invalid batch update record")
	}
	if type_ == BatchUpdateTypeUserQuota || type_ == BatchUpdateTypeTokenQuota {
		if DB != nil && DB.Migrator().HasTable(&QuotaWriterEpoch{}) {
			mode, err := currentQuotaWriterMode(DB)
			if err != nil || mode != QuotaWriterModeLegacy {
				return ErrLegacyQuotaWriterModeDisabled
			}
		}
		if DB == nil || !DB.Migrator().HasTable(&QuotaBalanceBatchDrain{}) {
			return errors.New("quota balance batch drain table is unavailable")
		}
	}
	batchUpdateLocks[type_].Lock()
	current := batchUpdateStores[type_][id]
	target := int64(0)
	var err error
	if type_ == BatchUpdateTypeRequestCount {
		const maxInt64 = int64(^uint64(0) >> 1)
		const minInt64 = -maxInt64 - 1
		if (value > 0 && current > maxInt64-value) || (value < 0 && current < minInt64-value) {
			err = errors.New("batch request count exceeds the int64 boundary")
		} else {
			target = current + value
		}
	} else {
		target, err = common.CheckedQuotaBalance(current, value)
	}
	if err != nil {
		batchUpdateLocks[type_].Unlock()
		return fmt.Errorf("batch update aggregate is out of range: %w", err)
	}
	batchUpdateStores[type_][id] = target
	batchUpdateLocks[type_].Unlock()
	if type_ == BatchUpdateTypeUserQuota || type_ == BatchUpdateTypeTokenQuota {
		if err := persistQuotaBalanceBatchGeneration(); err != nil {
			return err
		}
	}
	return nil
}

func batchUpdate() {
	durableBalanceDrain := DB != nil && DB.Migrator().HasTable(&QuotaBalanceBatchDrain{})
	if durableBalanceDrain {
		if err := persistQuotaBalanceBatchGeneration(); err != nil {
			common.SysLog("failed to persist quota balance batch drain: " + err.Error())
		}
		flushQuotaBalanceBatchGenerations()
	}

	// check if there's any data to update
	hasData := false
	for i := 0; i < BatchUpdateTypeCount; i++ {
		if durableBalanceDrain && (i == BatchUpdateTypeUserQuota || i == BatchUpdateTypeTokenQuota) {
			continue
		}
		batchUpdateLocks[i].Lock()
		if len(batchUpdateStores[i]) > 0 {
			hasData = true
			batchUpdateLocks[i].Unlock()
			break
		}
		batchUpdateLocks[i].Unlock()
	}

	if !hasData {
		return
	}

	common.SysLog("batch update started")
	stores := make([]map[int]int64, BatchUpdateTypeCount)
	for i := 0; i < BatchUpdateTypeCount; i++ {
		if durableBalanceDrain && (i == BatchUpdateTypeUserQuota || i == BatchUpdateTypeTokenQuota) {
			stores[i] = make(map[int]int64)
			continue
		}
		batchUpdateLocks[i].Lock()
		stores[i] = batchUpdateStores[i]
		batchUpdateStores[i] = make(map[int]int64)
		batchUpdateLocks[i].Unlock()
	}

	for i, store := range stores {
		if i == BatchUpdateTypeUserQuota || i == BatchUpdateTypeUsedQuota || i == BatchUpdateTypeRequestCount {
			continue
		}
		for key, value := range store {
			switch i {
			case BatchUpdateTypeTokenQuota:
				err := increaseTokenQuota(key, value)
				if err != nil {
					common.SysLog("failed to batch update token quota: " + err.Error())
				}
			case BatchUpdateTypeChannelUsedQuota:
				updateChannelUsedQuota(key, value)
			}
		}
	}

	userQuotaStore := stores[BatchUpdateTypeUserQuota]
	usedQuotaStore := stores[BatchUpdateTypeUsedQuota]
	requestCountStore := stores[BatchUpdateTypeRequestCount]

	userIDs := make(map[int]struct{}, len(userQuotaStore)+len(usedQuotaStore)+len(requestCountStore))
	for key := range userQuotaStore {
		userIDs[key] = struct{}{}
	}
	for key := range usedQuotaStore {
		userIDs[key] = struct{}{}
	}
	for key := range requestCountStore {
		userIDs[key] = struct{}{}
	}
	for key := range userIDs {
		updateUserQuotaUsedQuotaAndRequestCount(key, userQuotaStore[key], usedQuotaStore[key], requestCountStore[key])
	}
	common.SysLog("batch update finished")
}

func RecordExist(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return false, err
}

func shouldUpdateRedis(fromDB bool, err error) bool {
	return common.RedisEnabled && fromDB && err == nil
}
