package syncer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFatalError(t *testing.T) {
	s := &Syncer{}

	// Fatal errors
	assert.True(t, s.isFatalError(&SyncerError{Code: ExitConfigError}))
	assert.True(t, s.isFatalError(&SyncerError{Code: ExitValidationError}))
	assert.True(t, s.isFatalError(&SyncerError{Code: ExitSchemaMismatch}))

	// Non-fatal errors
	assert.False(t, s.isFatalError(&SyncerError{Code: ExitSyncFailure}))
	assert.False(t, s.isFatalError(&SyncerError{Code: ExitGeneralFailure}))
	assert.False(t, s.isFatalError(&SyncerError{Code: ExitConnectionError}))
	assert.False(t, s.isFatalError(errors.New("random error")))
	assert.False(t, s.isFatalError(nil))
}
