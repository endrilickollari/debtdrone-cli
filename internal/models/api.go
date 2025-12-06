package models

type ErrorResponse struct {
	Error string `json:"error"`
}

type SuccessResponse struct {
	Message string `json:"message"`
}
type ActivityRequest struct {
	ActivityType string                 `json:"activity_type" binding:"required"`
	ResourceType *string                `json:"resource_type,omitempty"`
	ResourceID   *string                `json:"resource_id,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type SnapshotRequest struct {
	RepositoryID string `json:"repository_id" binding:"required"`
}
