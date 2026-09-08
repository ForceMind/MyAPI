package dto

const TaskOperationObject = "task_operation"

// TaskOperationResponse is the complete public representation of a durable
// task submission operation. Owner, credential, billing, recovery and
// provider details are deliberately excluded from this contract.
type TaskOperationResponse struct {
	ID                string `json:"id"`
	Object            string `json:"object"`
	Kind              string `json:"kind"`
	Status            string `json:"status"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
	DispatchStartedAt *int64 `json:"dispatch_started_at"`
	ResolvedAt        *int64 `json:"resolved_at"`
}
