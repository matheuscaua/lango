package domain

import "errors"

var (
	// Consumer errors
	ErrConsumerNotFound = errors.New("consumer not found")
	ErrInvalidAPIKey    = errors.New("invalid api key")
	ErrConsumerInactive = errors.New("consumer is inactive")

	// Integration errors
	ErrIntegrationNotFound = errors.New("integration not found")
	// ErrIntegrationNotOwned means the integration exists but belongs to a
	// different consumer than the one authenticated on this request — this is
	// the check that makes cross-consumer message leakage impossible.
	ErrIntegrationNotOwned = errors.New("integration does not belong to this consumer")

	// Message errors
	ErrInvalidPhoneNumber  = errors.New("phone number is not in E.164 format")
	ErrUnsupportedMessage  = errors.New("unsupported message type for this provider")
	ErrUnsupportedProvider = errors.New("unsupported provider")
)
