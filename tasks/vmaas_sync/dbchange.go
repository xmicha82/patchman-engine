package vmaas_sync //nolint:revive,stylecheck

import (
	"app/base"
	"app/base/types"
	"app/base/utils"
	"app/base/vmaas"
	"app/tasks"
	"net/http"

	"github.com/pkg/errors"
)

func isSyncNeeded(dbExportedTS *types.Rfc3339TimestampWithZ, vmaasExportedTS *types.Rfc3339Timestamp) bool {
	if dbExportedTS == nil || vmaasExportedTS == nil {
		return true
	}
	utils.LogInfo("last sync", dbExportedTS.Time(), "dbchange.exported", vmaasExportedTS.Time())
	return dbExportedTS.Time().Before(*vmaasExportedTS.Time())
}

func vmaasDBChangeRequest() (*vmaas.DBChangeResponse, error) {
	if vmaasClient == nil {
		panic("VMaaS client is nil")
	}

	vmaasCallFunc := func() (interface{}, *http.Response, error) {
		response := vmaas.DBChangeResponse{}
		resp, err := vmaasClient.Request(&base.Context, http.MethodGet, vmaasDBChangeURL, nil, &response)
		return &response, resp, err
	}

	vmaasDataPtr, err := utils.HTTPCallRetry(vmaasCallFunc, tasks.VmaasCallExpRetry, tasks.VmaasCallMaxRetries)
	if err != nil {
		vmaasCallCnt.WithLabelValues("error-dbchange").Inc()
		return nil, errors.Wrap(err, "Checking DBChange")
	}
	vmaasCallCnt.WithLabelValues("success").Inc()
	return vmaasDataPtr.(*vmaas.DBChangeResponse), nil
}

func VmaasDBExported() *types.Rfc3339Timestamp {
	dbchange, err := vmaasDBChangeRequest()
	if err != nil {
		utils.LogError("err", err, "Could'n query vmaas dbchange")
		return nil
	}
	return dbchange.GetExported()
}
