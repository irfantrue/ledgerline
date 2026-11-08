package cachetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"ledgerline/internal/shared/cache"
	"ledgerline/tests/mocks/cachemocks"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// newCache returns a cache backed by the mocked backend.
func newCache(backend *cachemocks.Backend) cache.Service {
	return cache.NewRedisCacheWithBackend(backend)
}

func TestSet(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Set", mock.Anything, "key", mock.Anything, time.Minute).Return(nil).Once()

	c := newCache(backend)

	require.NoError(t, c.Set(context.Background(), "key", "value", time.Minute))
	backend.AssertExpectations(t)
}

// TestSet_MarshalsTheValue checks that the value is stored as JSON, so a
// reader can decode it into a struct again.
func TestSet_MarshalsTheValue(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Set", mock.Anything, "key", []byte(`{"name":"alice"}`), mock.Anything).Return(nil).Once()

	c := newCache(backend)

	type payload struct {
		Name string `json:"name"`
	}

	require.NoError(t, c.Set(context.Background(), "key", payload{Name: "alice"}, time.Minute))
	backend.AssertExpectations(t)
}

// TestSet_UnmarshalableValue checks that a value that cannot be marshalled
// is reported instead of being stored in a broken shape.
func TestSet_UnmarshalableValue(t *testing.T) {
	backend := &cachemocks.Backend{}

	c := newCache(backend)

	// Channels cannot be marshalled to JSON.
	require.Error(t, c.Set(context.Background(), "key", make(chan int), time.Minute))
	backend.AssertNotCalled(t, "Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestGet(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Get", mock.Anything, "key").Return([]byte(`{"name":"alice"}`), nil).Once()

	c := newCache(backend)

	var got struct {
		Name string `json:"name"`
	}
	require.NoError(t, c.Get(context.Background(), "key", &got))
	require.Equal(t, "alice", got.Name)
	backend.AssertExpectations(t)
}

// TestGet_MissingKey checks that the error of the backend is passed on.
func TestGet_MissingKey(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Get", mock.Anything, "missing").Return(nil, errors.New("redis: nil")).Once()

	c := newCache(backend)

	var got string
	require.Error(t, c.Get(context.Background(), "missing", &got))
	backend.AssertExpectations(t)
}

// TestGet_InvalidPayload checks that a value that is not valid JSON for the
// destination type is reported.
func TestGet_InvalidPayload(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Get", mock.Anything, "key").Return([]byte("not json"), nil).Once()

	c := newCache(backend)

	var got struct {
		Name string `json:"name"`
	}
	require.Error(t, c.Get(context.Background(), "key", &got))
}

func TestDelete(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Del", mock.Anything, []string{"key"}).Return(nil).Once()

	c := newCache(backend)

	require.NoError(t, c.Delete(context.Background(), "key"))
	backend.AssertExpectations(t)
}

// TestDeleteByPrefix checks that only the keys with the prefix are removed.
// The scan runs until the cursor is back at zero, so the backend reports the
// keys in one round and then ends the scan.
func TestDeleteByPrefix(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Scan", mock.Anything, uint64(0), "prefix*", int64(100)).
		Return([]string{"prefix.1", "prefix.2"}, uint64(0), nil).
		Once()
	backend.On("Del", mock.Anything, []string{"prefix.1", "prefix.2"}).Return(nil).Once()

	c := newCache(backend)

	require.NoError(t, c.DeleteByPrefix(context.Background(), "prefix"))
	backend.AssertExpectations(t)
}

// TestDeleteByPrefix_ScansUntilTheCursorIsZero checks that the scan loop
// keeps going while the backend reports a next cursor.
func TestDeleteByPrefix_ScansUntilTheCursorIsZero(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Scan", mock.Anything, uint64(0), "prefix*", int64(100)).
		Return([]string{"prefix.1"}, uint64(7), nil).
		Once()
	backend.On("Scan", mock.Anything, uint64(7), "prefix*", int64(100)).
		Return([]string{"prefix.2"}, uint64(0), nil).
		Once()
	backend.On("Del", mock.Anything, []string{"prefix.1"}).Return(nil).Once()
	backend.On("Del", mock.Anything, []string{"prefix.2"}).Return(nil).Once()

	c := newCache(backend)

	require.NoError(t, c.DeleteByPrefix(context.Background(), "prefix"))
	backend.AssertExpectations(t)
}

// TestDeleteByPrefix_ScanError checks that a failing scan is reported and
// that no key is deleted afterwards.
func TestDeleteByPrefix_ScanError(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Scan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, uint64(0), errors.New("scan failed")).
		Once()

	c := newCache(backend)

	require.Error(t, c.DeleteByPrefix(context.Background(), "prefix"))
	backend.AssertNotCalled(t, "Del", mock.Anything, mock.Anything)
}

// TestDeleteByPrefix_DeleteError checks that a failing delete is reported.
func TestDeleteByPrefix_DeleteError(t *testing.T) {
	backend := &cachemocks.Backend{}
	backend.On("Scan", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]string{"prefix.1"}, uint64(0), nil).
		Once()
	backend.On("Del", mock.Anything, mock.Anything).Return(errors.New("del failed")).Once()

	c := newCache(backend)

	require.Error(t, c.DeleteByPrefix(context.Background(), "prefix"))
}
