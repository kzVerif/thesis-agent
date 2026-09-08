package download

import "time"

type Command struct {
	Type        string    `json:"type"`
	JobID       string    `json:"job_id"`
	FileID      string    `json:"file_id"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	DownloadURL string    `json:"download_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Status struct {
	Type    string `json:"type"`
	JobID   string `json:"job_id"`
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}
type Progress struct {
	Type            string `json:"type"`
	JobID           string `json:"job_id"`
	AgentID         string `json:"agent_id"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	TotalBytes      int64  `json:"total_bytes"`
	Progress        int    `json:"progress"`
}
type Result struct {
	Type            string `json:"type"`
	JobID           string `json:"job_id"`
	AgentID         string `json:"agent_id"`
	FileID          string `json:"file_id"`
	Status          string `json:"status"`
	BytesDownloaded int64  `json:"bytes_downloaded"`
	SHA256          string `json:"sha256,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
}

type ErrorCode string

const (
	InvalidCommand        ErrorCode = "INVALID_COMMAND"
	URLExpired            ErrorCode = "URL_EXPIRED"
	HTTPError             ErrorCode = "HTTP_ERROR"
	DownloadFailed        ErrorCode = "DOWNLOAD_FAILED"
	DiskWriteFailed       ErrorCode = "DISK_WRITE_FAILED"
	InsufficientDiskSpace ErrorCode = "INSUFFICIENT_DISK_SPACE"
	SizeMismatch          ErrorCode = "SIZE_MISMATCH"
	HashMismatch          ErrorCode = "HASH_MISMATCH"
	Cancelled             ErrorCode = "CANCELLED"
	InternalError         ErrorCode = "INTERNAL_ERROR"
)

type JobError struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *JobError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}
