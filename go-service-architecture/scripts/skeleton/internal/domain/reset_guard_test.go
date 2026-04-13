package domain

import (
	"errors"
	"testing"
)

func TestCheckResetAllowed(t *testing.T) {
	tests := []struct {
		name       string
		status     Status
		retryCount int
		retryLimit int
		wantErr    error
	}{
		{
			name:       "not_sent with retries remaining is rejected",
			status:     StatusNotSent,
			retryCount: 1,
			retryLimit: 3,
			wantErr:    ErrRetriesRemaining,
		},
		{
			name:       "not_sent with retries exhausted is allowed",
			status:     StatusNotSent,
			retryCount: 3,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "failed is always allowed",
			status:     StatusFailed,
			retryCount: 1,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "delivered is always allowed",
			status:     StatusDelivered,
			retryCount: 0,
			retryLimit: 3,
			wantErr:    nil,
		},
		{
			name:       "not_sent with zero retries remaining (edge: 0/0)",
			status:     StatusNotSent,
			retryCount: 0,
			retryLimit: 0,
			wantErr:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckResetAllowed(tt.status, tt.retryCount, tt.retryLimit)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("CheckResetAllowed() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
