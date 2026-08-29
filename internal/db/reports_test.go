package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_Reports_FilteringAndRead(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	r1 := ReportRecord{
		ActionType: "UPGRADE",
		ItemTitle:  "Movie 1",
		Status:     "SUCCESS",
	}
	r2 := ReportRecord{
		ActionType:   "UPGRADE",
		ItemTitle:    "Movie 2",
		Status:       "FAILED",
		ErrorMessage: "Network error",
		IsRead:       false,
	}
	r3 := ReportRecord{
		ActionType:   "DELETE",
		ItemTitle:    "Episode 1",
		Status:       "FAILED",
		ErrorMessage: "Delete error",
		IsRead:       true,
	}

	require.NoError(t, d.InsertReport(r1))
	require.NoError(t, d.InsertReport(r2))
	require.NoError(t, d.InsertReport(r3))

	t.Run("GetReportsFiltered all", func(t *testing.T) {
		reports, err := d.GetReportsFiltered("", 10, 0)
		require.NoError(t, err)
		assert.Len(t, reports, 3)
	})

	t.Run("GetReportsFiltered SUCCESS", func(t *testing.T) {
		reports, err := d.GetReportsFiltered("SUCCESS", 10, 0)
		require.NoError(t, err)
		assert.Len(t, reports, 1)
		assert.Equal(t, "SUCCESS", reports[0].Status)
	})

	t.Run("GetReportsFiltered FAILED", func(t *testing.T) {
		reports, err := d.GetReportsFiltered("FAILED", 10, 0)
		require.NoError(t, err)
		assert.Len(t, reports, 2)
	})

	t.Run("GetUnreadErrorsCount and MarkReportAsRead", func(t *testing.T) {
		unread, err := d.GetUnreadErrorsCount()
		require.NoError(t, err)
		assert.Equal(t, 1, unread) // Only r2 is FAILED and IsRead == false

		// Find r2's ID
		reports, err := d.GetReportsFiltered("FAILED", 10, 0)
		require.NoError(t, err)
		var unreadID int
		for _, r := range reports {
			if !r.IsRead {
				unreadID = r.ID
				break
			}
		}
		require.NotZero(t, unreadID)

		// Mark as read
		require.NoError(t, d.MarkReportAsRead(unreadID))

		unreadAfter, err := d.GetUnreadErrorsCount()
		require.NoError(t, err)
		assert.Equal(t, 0, unreadAfter)
	})

	t.Run("GetReportsAdvanced with action filter", func(t *testing.T) {
		reports, err := d.GetReportsAdvanced(ReportFilter{
			Action: "DELETE",
			Limit:  10,
		})
		require.NoError(t, err)
		assert.Len(t, reports, 1)
		assert.Equal(t, "DELETE", reports[0].ActionType)
	})

	t.Run("GetReportsAdvanced sorting by item title ASC/DESC", func(t *testing.T) {
		reportsAsc, err := d.GetReportsAdvanced(ReportFilter{
			SortBy:    "item",
			SortOrder: "asc",
			Limit:     10,
		})
		require.NoError(t, err)
		assert.Len(t, reportsAsc, 3)
		assert.Equal(t, "Episode 1", reportsAsc[0].ItemTitle)
		assert.Equal(t, "Movie 2", reportsAsc[2].ItemTitle)

		reportsDesc, err := d.GetReportsAdvanced(ReportFilter{
			SortBy:    "item",
			SortOrder: "desc",
			Limit:     10,
		})
		require.NoError(t, err)
		assert.Len(t, reportsDesc, 3)
		assert.Equal(t, "Movie 2", reportsDesc[0].ItemTitle)
		assert.Equal(t, "Episode 1", reportsDesc[2].ItemTitle)
	})

	t.Run("GetReportsAdvanced sorting by Net Saved", func(t *testing.T) {
		// r1: UPGRADE, SUCCESS, Before 10GB, After 4GB -> Saved 6GB
		// r4: DELETE, SUCCESS, Before 8GB -> Saved 8GB
		r4 := ReportRecord{
			ActionType:      "DELETE",
			ItemTitle:       "Large Movie",
			Status:          "SUCCESS",
			TotalSizeBefore: 8 * 1024 * 1024 * 1024,
		}
		require.NoError(t, d.InsertReport(r4))

		reports, err := d.GetReportsAdvanced(ReportFilter{
			SortBy:    "saved",
			SortOrder: "desc",
			Limit:     10,
		})
		require.NoError(t, err)
		require.NotEmpty(t, reports)
		assert.Equal(t, "Large Movie", reports[0].ItemTitle)
	})

	t.Run("GetDistinctReportActions", func(t *testing.T) {
		actions, err := d.GetDistinctReportActions()
		require.NoError(t, err)
		assert.Contains(t, actions, "UPGRADE")
		assert.Contains(t, actions, "DELETE")
	})
}
