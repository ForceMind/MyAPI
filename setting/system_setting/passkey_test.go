package system_setting

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerAddressAccessorsPreserveExactValues(t *testing.T) {
	original := GetServerAddress()
	t.Cleanup(func() { SetServerAddress(original) })

	for _, want := range []string{
		"https://login.example.test/path/",
		"",
		"  https://literal.example.test/  ",
	} {
		SetServerAddress(want)
		assert.Equal(t, want, GetServerAddress())
	}
}

func TestEffectivePasskeySettingsDerivationDoesNotPolluteRawExport(t *testing.T) {
	runtime := newRuntimeSettings("https://login.example.test:8443", PasskeySettings{
		Enabled:              true,
		RPDisplayName:        "My API",
		Origins:              "[]",
		UserVerification:     "required",
		AttachmentPreference: "platform",
	})
	setting := &managedPasskeySettings{runtime: runtime}

	before, err := setting.ExportConfigMap()
	require.NoError(t, err)

	effective := effectivePasskeySettings(runtime.current.Load())
	require.NotNil(t, effective)
	assert.Equal(t, "login.example.test:8443", effective.RPID)
	assert.Equal(t, "https://login.example.test:8443", effective.Origins)

	after, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Empty(t, after["rp_id"])
	assert.Equal(t, "[]", after["origins"])
}

func TestManagedPasskeySettingsInvalidUpdatePublishesNothing(t *testing.T) {
	runtime := newRuntimeSettings("https://login.example.test", PasskeySettings{
		Enabled:          true,
		RPDisplayName:    "Original",
		RPID:             "original.example.test",
		Origins:          "https://original.example.test",
		UserVerification: "preferred",
	})
	setting := &managedPasskeySettings{runtime: runtime}
	beforeSnapshot := runtime.current.Load()
	before, err := setting.ExportConfigMap()
	require.NoError(t, err)

	invalid := map[string]string{
		"enabled":         "not-a-bool",
		"rp_display_name": "Must not be published",
	}
	require.Error(t, setting.ValidateConfigMap(invalid))
	assert.Same(t, beforeSnapshot, runtime.current.Load())
	require.Error(t, setting.UpdateConfigMap(invalid))
	assert.Same(t, beforeSnapshot, runtime.current.Load())

	after, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestManagedPasskeySettingsConcurrentPartialUpdatesDoNotLoseFields(t *testing.T) {
	runtime := newRuntimeSettings("https://login.example.test", defaultPasskeySettings)
	setting := &managedPasskeySettings{runtime: runtime}
	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start
		errors <- setting.UpdateConfigMap(map[string]string{"rp_display_name": "Updated RP"})
	}()
	go func() {
		defer workers.Done()
		<-start
		errors <- setting.UpdateConfigMap(map[string]string{"attachment_preference": "platform"})
	}()

	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	exported, err := setting.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, "Updated RP", exported["rp_display_name"])
	assert.Equal(t, "platform", exported["attachment_preference"])
	assert.Equal(t, "preferred", exported["user_verification"])
}

func TestManagedPasskeySettingsExportUpdateRoundTrip(t *testing.T) {
	source := &managedPasskeySettings{runtime: newRuntimeSettings("https://source.example.test", PasskeySettings{
		Enabled:              true,
		RPDisplayName:        "Source RP",
		RPID:                 "source.example.test",
		Origins:              "https://source.example.test,https://admin.example.test",
		AllowInsecureOrigin:  true,
		UserVerification:     "discouraged",
		AttachmentPreference: "cross-platform",
	})}

	exported, err := source.ExportConfigMap()
	require.NoError(t, err)

	target := &managedPasskeySettings{runtime: newRuntimeSettings("https://target.example.test", defaultPasskeySettings)}
	require.NoError(t, target.UpdateConfigMap(exported))
	roundTripped, err := target.ExportConfigMap()
	require.NoError(t, err)
	assert.Equal(t, exported, roundTripped)
}

func TestRuntimeSettingsPublishesServerAddressAndPasskeyTogether(t *testing.T) {
	runtime := newRuntimeSettings("https://before.example.test", PasskeySettings{
		Enabled:          true,
		RPDisplayName:    "Before",
		RPID:             "before.example.test",
		Origins:          "https://before.example.test",
		UserVerification: "preferred",
	})
	before := runtime.current.Load()
	address := "https://after.example.test/"

	require.NoError(t, runtime.updatePasskeyAndServerAddress(&address, map[string]string{
		"rp_display_name":   "After",
		"rp_id":             "after.example.test",
		"origins":           "https://after.example.test",
		"user_verification": "required",
	}))

	after := runtime.current.Load()
	require.NotNil(t, after)
	assert.NotSame(t, before, after)
	assert.Equal(t, address, after.serverAddress)
	assert.Equal(t, "After", after.passkeySettings.RPDisplayName)
	assert.Equal(t, "after.example.test", after.passkeySettings.RPID)
	assert.Equal(t, "https://after.example.test", after.passkeySettings.Origins)
	assert.Equal(t, "required", after.passkeySettings.UserVerification)
}

func TestEffectivePasskeySettingsReadUsesOneValidGeneration(t *testing.T) {
	runtime := newRuntimeSettings("https://login.example.test", PasskeySettings{
		RPDisplayName:    "First generation",
		RPID:             "first.example.test",
		Origins:          "https://first.example.test",
		UserVerification: "preferred",
	})
	setting := &managedPasskeySettings{runtime: runtime}
	start := make(chan struct{})
	readResult := make(chan *PasskeySettings, 1)
	updateErr := make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start
		updateErr <- setting.UpdateConfigMap(map[string]string{
			"rp_display_name":       "Second generation",
			"rp_id":                 "second.example.test",
			"origins":               "https://second.example.test",
			"user_verification":     "required",
			"attachment_preference": "platform",
		})
	}()
	go func() {
		defer workers.Done()
		<-start
		readResult <- effectivePasskeySettings(runtime.current.Load())
	}()

	close(start)
	workers.Wait()
	require.NoError(t, <-updateErr)

	observed := <-readResult
	require.NotNil(t, observed)
	switch observed.RPDisplayName {
	case "First generation":
		assert.Equal(t, "first.example.test", observed.RPID)
		assert.Equal(t, "https://first.example.test", observed.Origins)
		assert.Equal(t, "preferred", observed.UserVerification)
	case "Second generation":
		assert.Equal(t, "second.example.test", observed.RPID)
		assert.Equal(t, "https://second.example.test", observed.Origins)
		assert.Equal(t, "required", observed.UserVerification)
		assert.Equal(t, "platform", observed.AttachmentPreference)
	default:
		t.Fatalf("unexpected passkey settings generation: %#v", observed)
	}
}
