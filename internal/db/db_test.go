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

func TestDB_ClearTable(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// Add test data to scan_state
	err = d.SetLastItemID("inst1", "100")
	require.NoError(t, err)
	err = d.SetLastItemID("inst2", "200")
	require.NoError(t, err)

	counts, err := d.GetTableCounts()
	require.NoError(t, err)
	assert.Equal(t, int64(2), counts["scan_state"])

	// Clear valid table
	rows, err := d.ClearTable("scan_state")
	require.NoError(t, err)
	assert.Equal(t, int64(2), rows)

	countsAfter, err := d.GetTableCounts()
	require.NoError(t, err)
	assert.Equal(t, int64(0), countsAfter["scan_state"])

	// Clear invalid table (SQL injection check / disallowed table)
	_, err = d.ClearTable("non_existent_table; DROP TABLE users;")
	assert.Error(t, err)
}

func TestDB_GetDashboardStats_UpgradeTypes(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// Normal UPGRADE: 10GB -> 4GB = 6GB saved
	require.NoError(t, d.InsertReport(ReportRecord{
		ActionType:      "UPGRADE",
		ItemTitle:       "Movie 1",
		Status:          "SUCCESS",
		TotalSizeBefore: 10 * 1024 * 1024 * 1024,
		TotalSizeAfter:  4 * 1024 * 1024 * 1024,
	}))

	// SEASON_UPGRADE: 20GB -> 15GB = 5GB saved
	require.NoError(t, d.InsertReport(ReportRecord{
		ActionType:      "SEASON_UPGRADE",
		ItemTitle:       "Show S01",
		Status:          "SUCCESS",
		TotalSizeBefore: 20 * 1024 * 1024 * 1024,
		TotalSizeAfter:  15 * 1024 * 1024 * 1024,
	}))

	// SERIES_UPGRADE: 50GB -> 35GB = 15GB saved
	require.NoError(t, d.InsertReport(ReportRecord{
		ActionType:      "SERIES_UPGRADE",
		ItemTitle:       "Complete Show",
		Status:          "SUCCESS",
		TotalSizeBefore: 50 * 1024 * 1024 * 1024,
		TotalSizeAfter:  35 * 1024 * 1024 * 1024,
	}))

	// DELETE: 8GB saved
	require.NoError(t, d.InsertReport(ReportRecord{
		ActionType:      "DELETE",
		ItemTitle:       "Deleted Movie",
		Status:          "SUCCESS",
		TotalSizeBefore: 8 * 1024 * 1024 * 1024,
	}))

	stats, err := d.GetDashboardStats()
	require.NoError(t, err)
	// Expected: 6GB + 5GB + 15GB + 8GB = 34GB
	expectedSaved := int64(34 * 1024 * 1024 * 1024)
	assert.Equal(t, expectedSaved, stats.TotalSpaceSaved)
}

func TestDB_CheckpointWAL(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test_wal.db"
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	walFrames, checkpointed, blocked, err := d.CheckpointWAL()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, walFrames, 0)
	assert.GreaterOrEqual(t, checkpointed, 0)
	assert.False(t, blocked)
}
