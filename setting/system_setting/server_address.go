package system_setting

const defaultServerAddress = "http://localhost:3000"

// GetServerAddress returns one complete runtime address generation. Callers
// that formerly read the exported ServerAddress variable must migrate to this
// accessor; the mutable variable was intentionally removed for race safety.
func GetServerAddress() string {
	if snapshot := systemRuntimeSettings.current.Load(); snapshot != nil {
		return snapshot.serverAddress
	}
	return defaultServerAddress
}

// SetServerAddress publishes the supplied address as a complete runtime
// generation. Callers that formerly assigned ServerAddress must migrate to
// this setter. It intentionally preserves the existing string semantics,
// including empty values and trailing slashes.
func SetServerAddress(serverAddress string) {
	_ = systemRuntimeSettings.updatePasskeyAndServerAddress(&serverAddress, nil)
}

// UpdatePasskeyAndServerAddress atomically publishes a ServerAddress update
// together with raw Passkey option fields. It is used by the option
// persistence boundary after validation and a successful database commit so
// readers never observe a new address paired with an old Passkey generation.
// A nil serverAddress leaves the current address unchanged.
func UpdatePasskeyAndServerAddress(serverAddress *string, passkeyValues map[string]string) error {
	return systemRuntimeSettings.updatePasskeyAndServerAddress(serverAddress, passkeyValues)
}
