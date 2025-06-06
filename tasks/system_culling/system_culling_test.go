package system_culling //nolint:revive,stylecheck

import (
	"app/base/core"
	"app/base/database"
	"app/base/models"
	"app/base/types"
	"app/base/utils"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var staleDate, _ = time.Parse(types.Rfc3339NoTz, "2006-01-02T15:04:05-07:00")

func TestSingleSystemStale(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()

	var oldAffected int
	var systems []models.SystemPlatform
	var accountData []models.AdvisoryAccountData

	database.DebugWithCachesCheck("stale-trigger", func() {
		assert.NotNil(t, staleDate)
		assert.NoError(t, database.DB.Find(&accountData, "systems_installable > 1 ").
			Order("systems_installable DESC").Error)
		assert.NoError(t, database.DB.Find(&systems,
			"rh_account_id = ? AND stale = false AND installable_advisory_count_cache > 0",
			accountData[0].RhAccountID).Order("id").Error)

		systems[0].StaleTimestamp = &staleDate
		systems[0].StaleWarningTimestamp = &staleDate
		assert.NoError(t, database.DB.Save(&systems[0]).Error)

		nMarked, err := markSystemsStale(database.DB, 0)
		assert.Nil(t, err)
		assert.Equal(t, 0, nMarked)

		nMarked, err = markSystemsStale(database.DB, 1)
		assert.Nil(t, err)
		assert.Equal(t, 1, nMarked)

		oldAffected = accountData[0].SystemsInstallable
		assert.NoError(t, database.DB.Find(&accountData, "rh_account_id = ? AND advisory_id = ?",
			accountData[0].RhAccountID, accountData[0].AdvisoryID).Error)

		assert.Equal(t, oldAffected-1, accountData[0].SystemsInstallable,
			"Systems affected should be decremented by one")
	})

	database.DebugWithCachesCheck("stale-trigger", func() {
		systems[0].StaleTimestamp = nil
		systems[0].StaleWarningTimestamp = nil
		systems[0].Stale = false
		assert.NoError(t, database.DB.Save(&systems[0]).Error)
		assert.NoError(t, database.DB.Find(&accountData, "rh_account_id = ? AND advisory_id = ?",
			accountData[0].RhAccountID, accountData[0].AdvisoryID).Error)

		assert.Equal(t, oldAffected, accountData[0].SystemsInstallable,
			"Systems affected should be changed to match value at the start of the test case")
	})
}

// Test for making sure system culling works
func TestMarkSystemsStale(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()

	var systems []models.SystemPlatform
	var accountData []models.AdvisoryAccountData
	assert.NotNil(t, staleDate)
	assert.NoError(t, database.DB.Find(&systems).Error)
	assert.NoError(t, database.DB.Find(&accountData).Error)
	for i := range systems {
		assert.NotEqual(t, 0, systems[i].ID)
		// Check for valid state before modifying the systems in DB
		assert.Equal(t, false, systems[i].Stale, "No systems should be stale")
		systems[i].StaleTimestamp = &staleDate
		systems[i].StaleWarningTimestamp = &staleDate
	}

	assert.True(t, len(accountData) > 0, "We should have some systems affected by advisories")
	for _, a := range accountData {
		assert.True(t, a.SystemsInstallable+a.SystemsApplicable > 0, "We should have some systems affected")
	}
	for i := range systems {
		assert.NoError(t, database.DB.Save(&systems[i]).Error)
	}
	nMarked, err := markSystemsStale(database.DB, 500)
	assert.Nil(t, err)
	assert.Equal(t, 18, nMarked)

	assert.NoError(t, database.DB.Find(&systems).Error)
	for i, s := range systems {
		assert.Equal(t, true, s.Stale, "All systems should be stale")
		s.StaleTimestamp = nil
		s.StaleWarningTimestamp = nil
		s.Stale = false
		systems[i] = s
	}

	assert.NoError(t, database.DB.Find(&accountData).Error)
	sumAffected := 0
	for _, a := range accountData {
		sumAffected += a.SystemsInstallable + a.SystemsApplicable
	}
	assert.True(t, sumAffected == 0, "all advisory_data should be deleted", sumAffected)
}

func TestMarkSystemsNotStale(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()

	var systems []models.SystemPlatform
	var accountData []models.AdvisoryAccountData

	assert.NoError(t, database.DB.Find(&systems).Error)
	for i, s := range systems {
		assert.Equal(t, true, s.Stale, "All systems should be stale at the start of the test")
		s.StaleTimestamp = nil
		s.StaleWarningTimestamp = nil
		s.Stale = false
		systems[i] = s
	}

	for i := range systems {
		assert.NoError(t, database.DB.Save(&systems[i]).Error)
	}

	assert.NoError(t, database.DB.Find(&accountData).Error)
	assert.True(t, len(accountData) > 0, "We should have some systems affected by advisories")
	for _, a := range accountData {
		assert.True(t, a.SystemsInstallable+a.SystemsApplicable > 0, "We should have some systems affected")
	}
}

func TestCullSystems(t *testing.T) {
	utils.SkipWithoutDB(t)
	utils.TestLoadEnv("conf/test.env")
	core.SetupTestEnvironment()
	utils.TestLoadEnv("conf/vmaas_sync.env")

	nToDelete := 4
	for i := 0; i < nToDelete; i++ {
		invID := fmt.Sprintf("00000000-0000-0000-0000-000000000de%d", i+1)
		assert.NoError(t, database.DB.Create(&models.SystemPlatform{
			InventoryID:     invID,
			RhAccountID:     1,
			DisplayName:     invID,
			CulledTimestamp: &staleDate,
		}).Error)
	}

	var cnt int64
	var cntAfter int64
	database.DebugWithCachesCheck("delete-culled", func() {
		assert.NoError(t, database.DB.Model(&models.SystemPlatform{}).Count(&cnt).Error)
		// first batch
		nDeleted, err := deleteCulledSystems(database.DB, 3)
		assert.Nil(t, err)
		assert.Equal(t, 3, nDeleted)

		// second batch
		nDeleted, err = deleteCulledSystems(database.DB, 3)
		assert.Nil(t, err)
		assert.Equal(t, 1, nDeleted)

		assert.NoError(t, database.DB.Model(&models.SystemPlatform{}).Count(&cntAfter).Error)
		assert.Equal(t, cnt-int64(nToDelete), cntAfter)
	})
}
