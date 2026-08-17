package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTroubleshootCmd_Summary(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	err = database.SetLastItemID("sonarr-1", "55")
	require.NoError(t, err)
	_ = database.Close()

	var buf bytes.Buffer
	runTroubleshootSummary(&buf, dbPath)

	output := buf.String()
	assert.Contains(t, output, "TABLE")
	assert.Contains(t, output, "COUNT")
	assert.Contains(t, output, "scan_state")
	assert.Contains(t, output, "1")
}

func TestTroubleshootCmd_ClearSingleTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	err = database.SetLastItemID("radarr-1", "99")
	require.NoError(t, err)
	_ = database.Close()

	var outBuf, errBuf bytes.Buffer
	runClearSingleTable(&outBuf, &errBuf, dbPath, "scan_state")

	assert.Contains(t, outBuf.String(), "Table \"scan_state\" cleared successfully. Deleted 1 rows.")
	assert.Empty(t, errBuf.String())

	// Check table is empty now
	database, err = db.Open(dbPath)
	require.NoError(t, err)
	counts, err := database.GetTableCounts()
	require.NoError(t, err)
	assert.Equal(t, int64(0), counts["scan_state"])
	_ = database.Close()
}

func TestTroubleshootCmd_ClearInvalidTable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	var outBuf, errBuf bytes.Buffer
	runClearSingleTable(&outBuf, &errBuf, dbPath, "invalid_table")

	assert.Contains(t, errBuf.String(), "Error clearing table \"invalid_table\"")
	assert.Empty(t, outBuf.String())
}

func TestTroubleshootCmd_ClearAllTables(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	err = database.SetLastItemID("sonarr-2", "123")
	require.NoError(t, err)
	_ = database.Close()

	var outBuf, errBuf bytes.Buffer
	runClearAllTables(&outBuf, &errBuf, dbPath)

	output := outBuf.String()
	assert.True(t, strings.Contains(output, "scan_state") || strings.Contains(output, "cleared"))
	assert.Empty(t, errBuf.String())
}
