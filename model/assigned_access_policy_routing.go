package model

// ReadAssignedAccessRoutingPolicy reuses the runtime candidate query without
// its lazy routing-schema initialization. Access previews must perform reads
// only, including when this is the first request after startup.
func ReadAssignedAccessRoutingPolicy(group, modelName, requestPath string) (ChannelRoutingPolicy, error) {
	candidates, err := queryChannelRoutingCandidates(group, modelName, requestPath)
	if err != nil {
		return ChannelRoutingPolicy{}, err
	}
	return BuildChannelRoutingPolicy(candidates), nil
}
