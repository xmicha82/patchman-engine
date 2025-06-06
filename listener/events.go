package listener

import (
	"app/base/database"
	"app/base/models"
	"app/base/mqueue"
	"app/base/utils"
	"time"

	"github.com/bytedance/sonic"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

const (
	WarnEmptyEventType = "empty event type received"
	WarnUnknownType    = "unknown event type received"
	WarnNoRowsModified = "no rows modified on delete event"
)

func EventsMessageHandler(m mqueue.KafkaMessage) error {
	var msgData map[string]interface{}
	utils.LogTrace("kafka message data", string(m.Value))
	if err := sonic.Unmarshal(m.Value, &msgData); err != nil {
		utils.LogError("msg", string(m.Value), "message is not a valid JSON")
		// Skip invalid messages
		return nil
	}
	if msgData["type"] == nil {
		utils.LogWarn("inventoryID", msgData["id"], WarnEmptyEventType)
		eventMsgsReceivedCnt.WithLabelValues("", ReceivedErrorOtherType).Inc()
		return nil
	}

	if enableBypass {
		utils.LogInfo("inventoryID", msgData["id"], "Processing bypassed")
		eventMsgsReceivedCnt.WithLabelValues(msgData["type"].(string), ReceivedBypassed).Inc()
		return nil
	}

	switch msgData["type"] {
	case "delete":
		var event mqueue.PlatformEvent
		if err := sonic.Unmarshal(m.Value, &event); err != nil {
			utils.LogError("inventoryID", msgData["id"], "msg", string(m.Value),
				"Invalid 'delete' message format")
		}
		return HandleDelete(event)
	case "updated":
		fallthrough
	case "created":
		var event HostEvent
		if err := sonic.Unmarshal(m.Value, &event); err != nil {
			utils.LogError("inventoryID", msgData["id"], "err", err, "msg", string(m.Value),
				"Invalid 'updated' message format")
			return nil
		}
		return HandleUpload(event)
	default:
		utils.LogWarn("msg", string(m.Value), WarnUnknownType)
		return nil
	}
}

func HandleDelete(event mqueue.PlatformEvent) error {
	tStart := time.Now()
	defer utils.ObserveSecondsSince(tStart, messageHandlingDuration.WithLabelValues(EventDelete))
	// TODO: Do we need locking here ?
	err := database.OnConflictUpdate(database.DB, "inventory_id", "when_deleted").
		Create(models.DeletedSystem{
			InventoryID: event.ID,
			WhenDeleted: time.Now(),
		}).Error

	if err != nil {
		utils.LogError("inventoryID", event.ID, "err", err.Error(), "Could not delete system")
		eventMsgsReceivedCnt.WithLabelValues(EventDelete, ReceivedErrorProcessing).Inc()
		return errors.Wrap(err, "Could not delete system")
	}

	query := database.DB.Exec("select deleted_inventory_id from delete_system(?::uuid)", event.ID)
	err = query.Error
	if err != nil {
		utils.LogError("inventoryID", event.ID, "err", err.Error(), "Could not delete system")
		eventMsgsReceivedCnt.WithLabelValues(EventDelete, ReceivedErrorProcessing).Inc()
		return errors.Wrap(err, "Could not opt_out system")
	}

	if query.RowsAffected == 0 {
		utils.LogWarn("inventoryID", event.ID, WarnNoRowsModified)
		eventMsgsReceivedCnt.WithLabelValues(EventDelete, ReceivedWarnNoRows).Inc()
		return nil
	}

	utils.LogInfo("inventoryID", event.ID, "count", query.RowsAffected, "Systems deleted")
	eventMsgsReceivedCnt.WithLabelValues(EventDelete, ReceivedSuccess).Inc()

	err = database.DB.
		Delete(&models.DeletedSystem{}, "when_deleted < ?", time.Now().Add(-deletionThreshold)).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		utils.LogWarn("inventoryID", event.ID, WarnNoRowsModified)
		return nil
	}
	return nil
}
