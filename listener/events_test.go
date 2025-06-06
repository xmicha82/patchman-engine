package listener

import (
	"app/base/core"
	"app/base/database"
	"app/base/models"
	"app/base/mqueue"
	"app/base/utils"
	"testing"

	"github.com/bytedance/sonic"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

const notexistid = "99c0ffee-0000-0000-0000-999999999999"

func TestUpdateSystem(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()
	configure()

	deleteData(t)
	assert.NoError(t, database.DB.Create(&models.SystemPlatform{
		InventoryID: id,
		RhAccountID: 1,
		DisplayName: id,
	}).Error)

	ev := createTestUploadEvent("1", id, "puptoo", false, false)
	name := "TEST_NAME"
	ev.Host.DisplayName = &name
	ev.Host.SystemProfile.InstalledPackages = &[]string{"kernel-0:4.18.0-193.1.2.el8_2.x86_64"}
	assert.NoError(t, HandleUpload(ev))

	var system models.SystemPlatform
	assert.NoError(t, database.DB.Order("ID DESC").Find(&system, "inventory_id = ?::uuid", id).Error)

	assert.Equal(t, name, system.DisplayName)
}

func TestDeleteSystem(t *testing.T) {
	deleteData(t)
	assert.NoError(t, database.DB.Create(&models.SystemPlatform{
		InventoryID: id,
		RhAccountID: 1,
		DisplayName: id,
	}).Error)

	deleteEvent := createTestDeleteEvent(id)
	err := HandleDelete(deleteEvent)
	assertSystemNotInDB(t)
	assert.NoError(t, err)
}

func TestDeleteSystemWarn1(t *testing.T) {
	logHook := utils.NewTestLogHook()
	log.AddHook(logHook)
	deleteEvent := createTestDeleteEvent(id)
	deleteEvent.Type = nil

	data, err := sonic.Marshal(deleteEvent)
	assert.NoError(t, err)

	err = EventsMessageHandler(mqueue.KafkaMessage{Value: data})
	assert.Equal(t, WarnEmptyEventType, logHook.LogEntries[len(logHook.LogEntries)-1].Message)

	assert.NoError(t, err)
}

func TestDeleteSystemWarn2(t *testing.T) {
	logHook := utils.NewTestLogHook()
	log.AddHook(logHook)
	deleteEvent := createTestDeleteEvent(id)
	nonDeleteType := "no-delete"
	deleteEvent.Type = &nonDeleteType

	data, err := sonic.Marshal(deleteEvent)
	assert.NoError(t, err)

	err = EventsMessageHandler(mqueue.KafkaMessage{Value: data})
	assert.Equal(t, WarnUnknownType, logHook.LogEntries[len(logHook.LogEntries)-1].Message)

	assert.NoError(t, err)
}

func TestDeleteSystemWarn3(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()
	configure()

	logHook := utils.NewTestLogHook()
	log.AddHook(logHook)

	deleteEvent := createTestDeleteEvent(notexistid)
	err := HandleDelete(deleteEvent)
	assert.NoError(t, err)

	assert.Equal(t, WarnNoRowsModified, logHook.LogEntries[len(logHook.LogEntries)-1].Message)
}

func TestUploadAfterDelete(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()
	configure()

	uploadEvent := createTestUploadEvent("1", id, "puptoo", true, false)
	err := HandleUpload(uploadEvent)
	assert.NoError(t, err)
	assertSystemNotInDB(t)
}

func TestDeleteCleanup(t *testing.T) {
	utils.SkipWithoutDB(t)
	core.SetupTestEnvironment()
	configure()

	err := database.DB.Exec("DELETE FROM deleted_system").Error
	assert.NoError(t, err)
}
