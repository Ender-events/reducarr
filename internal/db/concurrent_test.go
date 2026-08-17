package db

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentScanAndReads reproduce SQLite_BUSY error when
// a concurrent scan (writing in loop) is running while concurrent
// operations (auth, health) are also trying to access the database.
//
// Without fix (busy_timeout=0, journal DELETE) : errors "database is locked".
// With fix (busy_timeout + WAL mode)           : all operations succeed.
func TestConcurrentScanAndReads(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	d, err := Open(dbPath)
	require.NoError(t, err)
	defer func() {
		_ = d.Close()
		_ = os.Remove(dbPath)
	}()

	require.NoError(t, d.UpsertUser("admin", "password123"))

	const (
		scanIterations   = 500
		readerGoroutines = 10
	)

	var (
		writeErrors atomic.Int64
		readErrors  atomic.Int64
		wg          sync.WaitGroup
	)

	wg.Go(func() {
		for i := range scanIterations {
			rec := MediaFileRecord{
				ArrInstance:  "sonarr_0",
				ArrType:      "sonarr",
				ItemID:       int32(i),
				FileID:       int32(i),
				Path:         fmt.Sprintf("/media/show/s01e%04d.mkv", i),
				Title:        fmt.Sprintf("Show S01E%04d", i),
				Inode:        uint64(1000 + i),
				Size:         int64(5_000_000_000),
				Duration:     2700,
				Quality:      "WEBDL-1080p",
				SeasonNumber: 1,
			}
			if err := d.UpsertMediaFile(rec); err != nil {
				writeErrors.Add(1)
				t.Logf("WRITE ERROR (iter %d): %v", i, err)
			}
			if i%10 == 0 {
				if err := d.SetLastItemID("sonarr_0", fmt.Sprintf("%d", i)); err != nil {
					writeErrors.Add(1)
					t.Logf("SetLastItemID ERROR (iter %d): %v", i, err)
				}
			}
		}
	})

	// Keep the scan running before starting the readers
	time.Sleep(5 * time.Millisecond)

	for range readerGoroutines {
		wg.Go(func() {
			for range 20 {
				_, err := d.GetSession("non-existent-token")
				if err != nil && err.Error() != "sql: no rows in result set" {
					readErrors.Add(1)
					t.Logf("READ ERROR (GetSession): %v", err)
				}

				// multiple SELECTs, typical of /health endpoint
				_, err = d.GetDashboardStats()
				if err != nil {
					readErrors.Add(1)
					t.Logf("READ ERROR (GetDashboardStats): %v", err)
				}

				// POST /login during a scan
				ok, err := d.AuthenticateUser("admin", "password123")
				if err != nil {
					readErrors.Add(1)
					t.Logf("READ ERROR (AuthenticateUser): %v", err)
				} else if !ok {
					readErrors.Add(1)
					t.Logf("READ ERROR: AuthenticateUser returned false unexpectedly")
				}

				time.Sleep(time.Millisecond)
			}
		})
	}

	// --- simulating concurrent write goroutines (login session, candidate action)
	const writerGoroutines = 5
	var userWriteErrors atomic.Int64

	for w := range writerGoroutines {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := range 20 {
				token := fmt.Sprintf("token_%d_%d", workerID, i)
				err := d.CreateSession(token, "admin", time.Now().Add(time.Hour))
				if err != nil {
					userWriteErrors.Add(1)
					t.Logf("USER WRITE ERROR (CreateSession): %v", err)
				}

				// Grab / Report user action
				rep := &CandidateRecord{
					MediaFileRecord: MediaFileRecord{
						ArrInstance:  "sonarr_0",
						ArrType:      "sonarr",
						ItemID:       1,
						FileID:       int32(100 + i),
						Path:         fmt.Sprintf("/media/show/grab_%d_%d.mkv", workerID, i),
						Title:        "Test Grab",
						Inode:        9999,
						Size:         1000,
						Duration:     100,
						Quality:      "1080p",
						SeasonNumber: 1,
					},
					Reason: "User Grab action",
				}
				err = d.UpsertCandidate(rep.ArrInstance, rep.FileID, rep.Reason)
				if err != nil {
					userWriteErrors.Add(1)
					t.Logf("USER WRITE ERROR (UpsertCandidate): %v", err)
				}

				time.Sleep(2 * time.Millisecond)
			}
		}(w)
	}

	wg.Wait()

	t.Logf("Scan Write errors: %d / %d iterations", writeErrors.Load(), scanIterations)
	t.Logf("User Write errors: %d / %d user writes", userWriteErrors.Load(), writerGoroutines*20*2)
	t.Logf("Read       errors: %d / %d reads", readErrors.Load(), readerGoroutines*20*3)

	assert.Zero(t, writeErrors.Load(), "scan writes should not fail")
	assert.Zero(t, userWriteErrors.Load(), "user concurrent writes (Grab/Login) should not fail")
	assert.Zero(t, readErrors.Load(), "concurrent reads (auth/health) should not fail during scan")
}
