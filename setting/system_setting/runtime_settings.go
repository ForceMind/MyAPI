package system_setting

import (
	"sync"
	"sync/atomic"

	"github.com/ForceMind/MyAPI/setting/config"
)

// runtimeSnapshot is an immutable generation for settings whose readers must
// not observe a partially updated value. ServerAddress and the raw Passkey
// fields remain persisted independently; this snapshot only gives each reader
// a race-free, internally consistent runtime view.
type runtimeSnapshot struct {
	serverAddress   string
	passkeySettings PasskeySettings
}

type runtimeSettings struct {
	current    atomic.Pointer[runtimeSnapshot]
	writeMutex sync.Mutex
}

func newRuntimeSettings(serverAddress string, passkeySettings PasskeySettings) *runtimeSettings {
	settings := &runtimeSettings{}
	settings.current.Store(&runtimeSnapshot{
		serverAddress:   serverAddress,
		passkeySettings: passkeySettings,
	})
	return settings
}

var systemRuntimeSettings = newRuntimeSettings(defaultServerAddress, defaultPasskeySettings)

// updatePasskeyAndServerAddress publishes the related raw settings in one
// immutable generation. A nil serverAddress leaves the current address in
// place; absent Passkey keys leave their current raw fields in place.
func (s *runtimeSettings) updatePasskeyAndServerAddress(serverAddress *string, passkeyValues map[string]string) error {
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	candidate := runtimeSnapshot{serverAddress: defaultServerAddress, passkeySettings: defaultPasskeySettings}
	if current := s.current.Load(); current != nil {
		candidate = *current
	}
	if serverAddress != nil {
		candidate.serverAddress = *serverAddress
	}
	if len(passkeyValues) > 0 {
		if err := config.UpdateConfigFromMap(&candidate.passkeySettings, passkeyValues); err != nil {
			return err
		}
	}
	s.current.Store(&candidate)
	return nil
}
