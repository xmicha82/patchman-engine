package controllers

import (
	"app/base/models"
	"app/base/utils"
	"app/manager/middlewares"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var AdvisorySystemExportOpts = ListOpts{
	Fields: SystemsFields,
	// By default, we show only fresh systems. If all systems are required, you must pass in:true,false filter into the api
	DefaultFilters: map[string]FilterData{
		"stale": {
			Operator: "eq",
			Values:   []string{"false"},
		},
	},
	DefaultSort:  "-last_upload",
	StableSort:   "sp.id",
	SearchFields: []string{"sp.display_name"},
}

// @Summary Export systems for my account
// @Description  Export systems for my account. Export endpoints are not paginated.
// @ID exportAdvisorySystems
// @Security RhIdentity
// @Accept   json
// @Produce  json,text/csv
// @Param    advisory_id    path    string  true    "Advisory ID"
// @Param    search         query   string  false   "Find matching text"
// @Param    filter[id]              query   string  false "Filter"
// @Param    filter[display_name]    query   string  false "Filter"
// @Param    filter[stale]           query   string  false "Filter"
// @Param    filter[group_name] 									query []string 	false "Filter systems by inventory groups"
// @Param    filter[system_profile][sap_system]						query bool  	false "Filter only SAP systems"
// @Param    filter[system_profile][sap_sids]						query []string  false "Filter systems by their SAP SIDs"
// @Param    filter[system_profile][ansible]						query string 	false "Filter systems by ansible"
// @Param    filter[system_profile][ansible][controller_version]	query string 	false "Filter systems by ansible version"
// @Param    filter[system_profile][mssql]							query string 	false "Filter systems by mssql version"
// @Param    filter[system_profile][mssql][version]					query string 	false "Filter systems by mssql version"
// @Param    filter[os]              query   string    false "Filter OS version"
// @Param    tags                    query   []string  false "Tag filter"
// @Success 200 {array} AdvisorySystemDBLookup
// @Failure 400 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 415 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /export/advisories/{advisory_id}/systems [get]
func AdvisorySystemsExportHandler(c *gin.Context) {
	account := c.GetInt(utils.KeyAccount)
	groups := c.GetStringMapString(utils.KeyInventoryGroups)

	advisoryName := c.Param("advisory_id")
	if advisoryName == "" {
		c.JSON(http.StatusBadRequest, utils.ErrorResponse{Error: "advisory_id param not found"})
		return
	}

	db := middlewares.DBFromContext(c)
	var exists int64
	err := db.Model(&models.AdvisoryMetadata{}).
		Where("name = ? ", advisoryName).Count(&exists).Error
	if err != nil {
		LogAndRespError(c, err, "database error")
	}
	if exists == 0 {
		LogAndRespNotFound(c, errors.New("advisory not found"), "Advisory not found")
		return
	}

	query := buildAdvisorySystemsQuery(db, account, groups, advisoryName)
	filters, err := ParseAllFilters(c, AdvisorySystemExportOpts)
	if err != nil {
		return
	} // Error handled in method itself
	query, _ = ApplyInventoryFilter(filters, query, "sp.inventory_id")

	query = query.Order("sp.id")
	query, err = ExportListCommon(query, c, AdvisorySystemExportOpts)
	if err != nil {
		return
	} // Error handled in method itself

	outputExportData(c, query)
}

func outputExportData(c *gin.Context, query *gorm.DB) {
	var systems []AdvisorySystemDBLookup
	err := query.Find(&systems).Error
	if err != nil {
		LogAndRespError(c, err, "db error")
		return
	}

	OutputExportData(c, systems)
}
