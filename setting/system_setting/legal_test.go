package system_setting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManagedLegalSettingsExportsDefaultsAndPartialUpdatesPreserveTheCandidate(t *testing.T) {
	state := newManagedLegalSettings(defaultLegalSettings)

	exported, err := state.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"user_agreement": "",
		"privacy_policy": "",
	}, exported)

	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"user_agreement": "Agreement v1",
	}))
	require.NoError(t, state.UpdateConfigMap(map[string]string{
		"privacy_policy": "Privacy v1",
	}))

	assert.Equal(t, LegalSettings{
		UserAgreement: "Agreement v1",
		PrivacyPolicy: "Privacy v1",
	}, state.snapshot())
}

func TestManagedLegalSettingsValidationAndDetachedSnapshotsDoNotPublish(t *testing.T) {
	state := newManagedLegalSettings(LegalSettings{
		UserAgreement: "Agreement v1",
		PrivacyPolicy: "Privacy v1",
	})
	before := state.current.Load()

	require.NoError(t, state.ValidateConfigMap(map[string]string{
		"user_agreement": "Candidate agreement",
		"privacy_policy": "Candidate privacy",
	}))
	assert.Same(t, before, state.current.Load())
	assert.Equal(t, LegalSettings{
		UserAgreement: "Agreement v1",
		PrivacyPolicy: "Privacy v1",
	}, state.snapshot())

	snapshot := state.detachedSnapshot()
	snapshot.UserAgreement = "Caller-local value"
	assert.Equal(t, "Agreement v1", state.snapshot().UserAgreement)
}

func TestManagedLegalSettingsConcurrentPartialUpdatesRetainBothFields(t *testing.T) {
	state := newManagedLegalSettings(defaultLegalSettings)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup

	for _, update := range []map[string]string{
		{"user_agreement": "Agreement v1"},
		{"privacy_policy": "Privacy v1"},
	} {
		update := update
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			errs <- state.UpdateConfigMap(update)
		}()
	}

	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	assert.Equal(t, LegalSettings{
		UserAgreement: "Agreement v1",
		PrivacyPolicy: "Privacy v1",
	}, state.snapshot())
}
