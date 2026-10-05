package common

// These opt-in limits do not change the legacy RELAY_TIMEOUT contract.
var RelayFailoverTimeoutSeconds int
var RelayFailureCooldownSeconds int

func LoadRelaySchedulingLimits() {
	RelayFailoverTimeoutSeconds = boundedRelaySchedulingEnv("RELAY_FAILOVER_TIMEOUT_SECONDS", 3600)
	RelayFailureCooldownSeconds = boundedRelaySchedulingEnv("RELAY_FAILURE_COOLDOWN_SECONDS", 300)
}

func boundedRelaySchedulingEnv(name string, maximum int) int {
	value := GetEnvOrDefault(name, 0)
	if value < 0 || value > maximum {
		SysError(name + " is outside its supported range; disabled")
		return 0
	}
	return value
}
