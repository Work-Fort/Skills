package domain

// CheckResetAllowed returns ErrRetriesRemaining if the notification is
// in not_sent state and retry_count < retry_limit (auto-retry still in
// progress). For failed or delivered notifications, reset is always
// allowed. This centralises the guard logic so both REST and MCP
// handlers use the same check (REQ-023, REQ-024, REQ-025).
func CheckResetAllowed(status Status, retryCount, retryLimit int) error {
	if status == StatusNotSent && retryCount < retryLimit {
		return ErrRetriesRemaining
	}
	return nil
}
