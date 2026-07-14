package db

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_Open_InMemory(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	assert.NotNil(t, d)
}

func TestDB_GetLastItemID_ReturnsEmptyWhenNotFound(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	lastID, err := d.GetLastItemID("non-existent-instance")
	assert.NoError(t, err)
	assert.Empty(t, lastID)
}

func TestDB_SetAndGetLastItemID(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	err = d.SetLastItemID("instance-1", "1234")
	assert.NoError(t, err)

	lastID, err := d.GetLastItemID("instance-1")
	assert.NoError(t, err)
	assert.Equal(t, "1234", lastID)
}

func TestDB_SetLastItemID_Upsert(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	err = d.SetLastItemID("instance-1", "100")
	assert.NoError(t, err)

	err = d.SetLastItemID("instance-1", "200")
	assert.NoError(t, err)

	lastID, err := d.GetLastItemID("instance-1")
	assert.NoError(t, err)
	assert.Equal(t, "200", lastID)
}

func TestDB_GetAllScanStates(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	err = d.SetLastItemID("instance-B", "200")
	assert.NoError(t, err)

	err = d.SetLastItemID("instance-A", "100")
	assert.NoError(t, err)

	states, err := d.GetAllScanStates()
	assert.NoError(t, err)
	require.Len(t, states, 2)

	assert.Equal(t, "instance-A", states[0].InstanceID)
	assert.Equal(t, "100", states[0].LastItemID)

	assert.Equal(t, "instance-B", states[1].InstanceID)
	assert.Equal(t, "200", states[1].LastItemID)
}

func TestDB_AcquireScanLock_FirstAcquisition(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	acquired, err := d.AcquireScanLock("lock-1", os.Getpid(), "scheduler", 60)
	assert.NoError(t, err)
	assert.True(t, acquired)
}

func TestDB_AcquireScanLock_SecondAcquisitionFails(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	acquired, err := d.AcquireScanLock("lock-1", os.Getpid(), "scheduler", 60)
	require.NoError(t, err)
	require.True(t, acquired)

	acquiredSecond, err := d.AcquireScanLock("lock-1", os.Getpid()+1, "manual", 60)
	assert.NoError(t, err)
	assert.False(t, acquiredSecond)
}

func TestDB_ReleaseScanLock_MakesLockAvailable(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	acquired, err := d.AcquireScanLock("lock-1", os.Getpid(), "scheduler", 60)
	require.NoError(t, err)
	require.True(t, acquired)

	err = d.ReleaseScanLock("lock-1")
	assert.NoError(t, err)

	acquiredAgain, err := d.AcquireScanLock("lock-1", os.Getpid()+1, "manual", 60)
	assert.NoError(t, err)
	assert.True(t, acquiredAgain)
}

func TestDB_GetDashboardStats_Empty(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	stats, err := d.GetDashboardStats()
	assert.NoError(t, err)
	assert.Equal(t, int64(0), stats.TotalSpaceSaved)
	assert.Equal(t, 0, stats.PendingCandidates)
	assert.Equal(t, 0, stats.IgnoredFiles)
	assert.Equal(t, 0, stats.FailedActions)
	assert.Equal(t, "Never", stats.LastScanTime)
}
