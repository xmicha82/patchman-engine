package listener

import (
	"app/base/database"
	"app/base/inventory"
	"app/base/models"
	"app/base/mqueue"
	"app/base/types"
	"app/base/utils"
	"app/base/vmaas"
	"app/manager/middlewares"
	"fmt"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/stretchr/testify/assert"
)

const id = "99c0ffee-0000-0000-0000-0000c0ffee99"

var s3URL = "http://platform:9001/yum_updates"

func TestInit(_ *testing.T) {
	utils.TestLoadEnv("conf/listener.env")
}

func deleteData(t *testing.T) {
	// Delete test data from previous run
	assert.Nil(t, database.DB.Unscoped().Exec("DELETE FROM advisory_account_data aad "+
		"USING rh_account ra WHERE ra.id = aad.rh_account_id AND ra.name = ?", id).Error)
	assert.Nil(t, database.DB.Unscoped().Where("first_reported > timestamp '2020-01-01'").
		Delete(&models.SystemAdvisories{}).Error)
	assert.Nil(t, database.DB.Unscoped().Where("repo_id NOT IN (1, 2) OR system_id NOT IN (2, 3, 17)").
		Delete(&models.SystemRepo{}).Error)
	assert.Nil(t, database.DB.Unscoped().Where("name NOT IN ('repo1', 'repo2', 'repo3', 'repo4')").
		Delete(&models.Repo{}).Error)
	assert.Nil(t, database.DB.Unscoped().Where("inventory_id = ?::uuid", id).Delete(&models.SystemPlatform{}).Error)
	assert.Nil(t, database.DB.Unscoped().Where("name = ?", id).Delete(&models.RhAccount{}).Error)
}

// nolint: unparam
func assertSystemInDB(t *testing.T, inventoryID string, rhAccountID *int, reporterID *int) {
	var system models.SystemPlatform
	assert.NoError(t, database.DB.Where("inventory_id = ?::uuid", inventoryID).Find(&system).Error)
	assert.Equal(t, system.InventoryID, inventoryID)

	var account models.RhAccount
	assert.NoError(t, database.DB.Where("id = ?", system.RhAccountID).Find(&account).Error)
	if account.Name == nil || *account.Name == "" {
		assert.Equal(t, inventoryID, *account.OrgID)
	} else {
		assert.Equal(t, inventoryID, *account.Name)
	}
	if rhAccountID != nil {
		assert.Equal(t, system.RhAccountID, *rhAccountID)
	}
	assert.Equal(t, system.ReporterID, reporterID)

	now := time.Now().Add(-time.Minute)
	assert.True(t, system.LastUpdated.After(now), "Last updated")
	assert.True(t, system.UnchangedSince.After(now), "Unchanged since")
	assert.True(t, system.LastUpload.After(now), "Last upload")
}

func assertSystemNotInDB(t *testing.T) {
	var systemCount int64
	assert.Nil(t, database.DB.Model(models.SystemPlatform{}).
		Where("inventory_id = ?::uuid", id).Count(&systemCount).Error)

	assert.Equal(t, int(systemCount), 0)
}

func getOrCreateTestAccount(t *testing.T) int {
	accountID, err := middlewares.GetOrCreateAccount(id)
	assert.Nil(t, err)
	return accountID
}

// nolint: unparam
func createTestUploadEvent(orgID, inventoryID, reporter string, packages, yum bool) HostEvent {
	now := time.Now()
	ev := HostEvent{
		Type: "created",
		Host: Host{
			ID:       inventoryID,
			OrgID:    &orgID,
			Reporter: reporter,
			PerReporterStaleness: map[string]inventory.ReporterStaleness{
				reporter: {LastCheckIn: types.Rfc3339TimestampWithZ(now)},
			},
		},
	}
	if packages {
		ev.Host.SystemProfile.InstalledPackages = &[]string{"kernel-0:54321-1.rhel8.x86_64"}
	}
	ev.Host.SystemProfile.DnfModules = &[]inventory.DnfModule{{
		Name:   "modName",
		Stream: "modStream"}}
	ev.Host.SystemProfile.YumRepos = &[]inventory.YumRepo{{ID: "repo1", Enabled: true}}
	if yum {
		ev.PlatformMetadata = HostPlatformMetadata{
			CustomMetadata: HostCustomMetadata{
				YumUpdates:      []byte(`{"kernel-0.3": {}}`),
				YumUpdatesS3URL: &s3URL,
			},
		}
	}
	return ev
}

func createTestDeleteEvent(inventoryID string) mqueue.PlatformEvent {
	typ := "delete"
	return mqueue.PlatformEvent{
		ID:   inventoryID,
		Type: &typ,
	}
}

func assertReposInDB(t *testing.T, repos []string) {
	var n []string
	err := database.DB.Model(&models.Repo{}).Where("name IN (?)", repos).Pluck("name", &n).Error
	fmt.Println(n)
	assert.Nil(t, err)
	assert.Equal(t, len(repos), len(n))
}

func assertSystemReposInDB(t *testing.T, systemID int64, repos []string) {
	var c int64

	err := database.DB.Table("repo r").
		Joins("JOIN system_repo sr on sr.repo_id = r.id and sr.system_id = ? ", systemID).
		Where("r.name in (?)", repos).
		Count(&c).Error
	assert.NoError(t, err)
	assert.Equal(t, c, int64(len(repos)))
}

func assertYumUpdatesInDB(t *testing.T, inventoryID string, yumUpdates *YumUpdates) {
	var system models.SystemPlatform
	assert.NoError(t, database.DB.Where("inventory_id = ?::uuid", inventoryID).Find(&system).Error)
	assert.Equal(t, system.InventoryID, inventoryID)
	var systemYumUpdatesParsed vmaas.UpdatesV3Response
	var yumUpdatesParsed vmaas.UpdatesV3Response
	err := sonic.Unmarshal(system.YumUpdates, &systemYumUpdatesParsed)
	assert.Nil(t, err)
	err = sonic.Unmarshal(yumUpdates.RawParsed, &yumUpdatesParsed)
	assert.Nil(t, err)
	assert.Equal(t, systemYumUpdatesParsed, yumUpdatesParsed)
	assert.Equal(t, yumUpdates.BuiltPkgcache, system.BuiltPkgcache)
}
