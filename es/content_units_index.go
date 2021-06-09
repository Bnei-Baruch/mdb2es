package es

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Bnei-Baruch/sqlboiler/queries/qm"
	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/integration"
	"github.com/Bnei-Baruch/archive-backend/mdb"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

func MakeContentUnitsIndex(namespace string, indexDate string, db *sql.DB, esc *elastic.Client) *ContentUnitsIndex {
	cui := new(ContentUnitsIndex)
	cui.resultType = consts.ES_RESULT_TYPE_UNITS
	cui.baseName = consts.ES_RESULTS_INDEX
	cui.namespace = namespace
	cui.indexDate = indexDate
	cui.db = db
	cui.esc = esc
	cui.assetsService = integration.NewAssetsService(unzipUrl)
	return cui
}

type ContentUnitsIndex struct {
	BaseIndex
	Progress uint64
	assetsService integration.AssetsService
}

func defaultContentUnit(cu *mdbmodels.ContentUnit) bool {
	return cu.Secure == 0 && cu.Published && !utils.Int64InSlice(cu.TypeID, []int64{
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LELO_MIKUD].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_PUBLICATION].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SONG].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BOOK].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BLOG_POST].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_KITEI_MAKOR].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_RESEARCH_MATERIAL].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_KTAIM_NIVCHARIM].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_UNKNOWN].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SOURCE].ID,
	})
}

func defaultContentUnitSql() string {
	return fmt.Sprintf("cu.secure = 0 AND cu.published IS TRUE AND cu.type_id NOT IN (%d, %d, %d, %d, %d, %d, %d, %d, %d, %d)",
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LELO_MIKUD].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_PUBLICATION].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SONG].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BOOK].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BLOG_POST].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_KITEI_MAKOR].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_RESEARCH_MATERIAL].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_KTAIM_NIVCHARIM].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_UNKNOWN].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SOURCE].ID,
	)
}

func (index *ContentUnitsIndex) ReindexAll() error {
	log.Info("Content Units Index - Reindex all.")
	_, indexErrors := index.RemoveFromIndexQuery(index.FilterByResultTypeQuery(index.resultType))
	if err := indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "ContentUnitsIndex"); err != nil {
		return err
	}
	return indexErrors.Join(index.addToIndexSql(defaultContentUnitSql()), "").CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "ContentUnitsIndex")
}

func (index *ContentUnitsIndex) RemoveFromIndex(scope Scope) (map[string][]string, error) {
	log.Debugf("Content Units Index - RemoveFromIndex. Scope: %+v.", scope)
	removed, indexErrors := index.removeFromIndex(scope)
	return removed, indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "ContentUnitsIndex")
}

func (index *ContentUnitsIndex) AddToIndex(scope Scope, removedUIDs []string) error {
	log.Debugf("ContentUnitsIndex - AddToIndex. Scope: %+v, removedUIDs: %+v", scope, removedUIDs)
	return index.addToIndex(scope, removedUIDs).CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "ContentUnitsIndex")
}

func (index *ContentUnitsIndex) addToIndex(scope Scope, removedUIDs []string) *IndexErrors {
	// TODO: Missing tag scope handling.
	sqlScope := defaultContentUnitSql()
	uids := removedUIDs
	if scope.ContentUnitUID != "" {
		uids = append(uids, scope.ContentUnitUID)
	}
	indexErrors := MakeIndexErrors()
	if scope.FileUID != "" {
		moreUIDs, err := contentUnitsScopeByFile(index.db, scope.FileUID)
		indexErrors.SetError(err)
		uids = append(uids, moreUIDs...)
	}
	if scope.CollectionUID != "" {
		moreUIDs, err := contentUnitsScopeByCollection(index.db, scope.CollectionUID)
		indexErrors.SetError(err)
		uids = append(uids, moreUIDs...)
	}
	if scope.SourceUID != "" {
		moreUIDs, err := contentUnitsScopeBySource(index.db, scope.SourceUID)
		indexErrors.SetError(err)
		uids = append(uids, moreUIDs...)
	}
	if len(uids) == 0 {
		return indexErrors
	}
	quoted := make([]string, len(uids))
	for i, uid := range uids {
		quoted[i] = fmt.Sprintf("'%s'", uid)
	}
	sqlScope = fmt.Sprintf("%s AND cu.uid IN (%s)", sqlScope, strings.Join(quoted, ","))
	return indexErrors.Join(index.addToIndexSql(sqlScope), fmt.Sprintf("Failed adding to index: %+v", sqlScope))
}

func (index *ContentUnitsIndex) removeFromIndex(scope Scope) (map[string][]string, *IndexErrors) {
	typedUids := make([]string, 0)
	if scope.ContentUnitUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_CONTENT_UNIT, scope.ContentUnitUID))
	}
	indexErrors := MakeIndexErrors()
	if scope.FileUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_FILE, scope.FileUID))
		moreUIDs, err := contentUnitsScopeByFile(index.db, scope.FileUID)
		indexErrors.SetError(err)
		typedUids = append(typedUids, KeyValues(consts.ES_UID_TYPE_CONTENT_UNIT, moreUIDs)...)
	}
	if scope.CollectionUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_COLLECTION, scope.CollectionUID))
		moreUIDs, err := contentUnitsScopeByCollection(index.db, scope.CollectionUID)
		indexErrors.SetError(err)
		typedUids = append(typedUids, KeyValues(consts.ES_UID_TYPE_CONTENT_UNIT, moreUIDs)...)
	}
	if scope.TagUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_TAG, scope.TagUID))
	}
	if scope.SourceUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_SOURCE, scope.SourceUID))
		moreUIDs, err := contentUnitsScopeBySource(index.db, scope.SourceUID)
		indexErrors.SetError(err)
		typedUids = append(typedUids, KeyValues(consts.ES_UID_TYPE_CONTENT_UNIT, moreUIDs)...)
	}
	if len(typedUids) > 0 {
		typedUidsI := make([]interface{}, len(typedUids))
		for i, typedUID := range typedUids {
			typedUidsI[i] = typedUID
		}
		elasticScope := index.FilterByResultTypeQuery(index.resultType).
			Filter(elastic.NewTermsQuery("typed_uids", typedUidsI...))
		uids, removeIndexErrors := index.RemoveFromIndexQuery(elasticScope)
		return uids, indexErrors.Join(removeIndexErrors, fmt.Sprintf("Failed removing from index: %+v", elasticScope))
	} else {
		// Nothing to remove, or error.
		return make(map[string][]string), indexErrors
	}
}

func (index *ContentUnitsIndex) bulkIndexUnits(bulk OffsetLimitJob, sqlScope string) *IndexErrors {
	indexErrors := MakeIndexErrors()
	var units []*mdbmodels.ContentUnit
	if err := mdbmodels.NewQuery(index.db,
		qm.From("content_units as cu"),
		qm.Load("ContentUnitI18ns"),
		qm.Load("CollectionsContentUnits"),
		qm.Load("CollectionsContentUnits.Collection"),
		qm.Where(sqlScope),
		qm.Offset(bulk.Offset),
		qm.Limit(bulk.Limit)).Bind(&units); err != nil {
		return indexErrors.SetError(errors.Wrap(err, "Fetch units from mdb"))
	}
	log.Infof("Content Units Index - Adding %d units (offset: %d total: %d).", len(units), bulk.Offset, bulk.Total)

	indexData, err := MakeIndexData(index.db, sqlScope)
	if err != nil {
		return indexErrors.SetError(errors.Wrap(err, "Failed making index data."))
	}
	i18nMap := make(map[string][]Result)
	for _, unit := range units {
		i18nUnit, unitIndexErrors := index.prepareIndexUnit(unit, indexData)
		for lang, result := range i18nUnit {
			i18nMap[lang] = append(i18nMap[lang], result)
		}
		indexErrors.Join(unitIndexErrors, "")
	}

	// Index each document in its language index
	for lang, results := range i18nMap {
		indexName := index.IndexName(lang)

		bulkService := elastic.NewBulkService(index.esc).Index(indexName)
		for _, result := range results {
			indexErrors.ShouldIndex(lang)
			bulkService.Add(elastic.NewBulkIndexRequest().Index(indexName).Type("result").Doc(result))
		}
		bulkRes, e := bulkService.Do(context.TODO())
		if e != nil {
			indexErrors.LanguageError(lang, e, fmt.Sprintf("Results Index - bulkIndexUnits %s %+v.", indexName, sqlScope))
			continue
		}
		for _, itemMap := range bulkRes.Items {
			for _, res := range itemMap {
				if res.Result == "created" {
					indexErrors.Indexed(lang)
				}
			}
		}
	}
	indexErrors.PrintIndexCounts(fmt.Sprintf("ContentUnitIndex %d - %d / %d", bulk.Offset, bulk.Offset+bulk.Limit, bulk.Total))
	return indexErrors
}

type OffsetLimitJob struct {
	Offset int
	Limit  int
	Total  int
}

func (index *ContentUnitsIndex) addToIndexSql(sqlScope string) *IndexErrors {
	indexErrors := MakeIndexErrors()
	var count int
	err := mdbmodels.NewQuery(index.db,
		qm.Select("COUNT(1)"),
		qm.From("content_units as cu"),
		qm.Where(sqlScope)).QueryRow().Scan(&count)
	if err != nil {
		return indexErrors.SetError(errors.Wrapf(err, "Failed fetching content_units with sql scope: %s", sqlScope))
	}

	log.Debugf("Content Units Index - Adding %d units. Scope: %s", count, sqlScope)

	tasks := make(chan OffsetLimitJob, 300)
	errors := make(chan *IndexErrors, 300)
	doneAdding := make(chan bool)

	tasksCount := 0
	go func() {
		offset := 0
		limit := utils.MaxInt(10, utils.MinInt(250, (int)(count/10)))
		for offset < count {
			tasks <- OffsetLimitJob{offset, limit, count}
			tasksCount += 1
			offset += limit
		}
		close(tasks)
		doneAdding <- true
	}()

	for w := 1; w <= 10; w++ {
		go func(tasks <-chan OffsetLimitJob, errors chan<- *IndexErrors) {
			for task := range tasks {
				errors <- index.bulkIndexUnits(task, sqlScope)
			}
		}(tasks, errors)
	}

	<-doneAdding
	for a := 1; a <= tasksCount; a++ {
		indexErrors.Join(<-errors, "")
	}

	return indexErrors
}

func collectionsContentTypes(collectionsContentUnits mdbmodels.CollectionsContentUnitSlice) []string {
	ret := make([]string, len(collectionsContentUnits))
	for i, ccu := range collectionsContentUnits {
		ret[i] = mdb.CONTENT_TYPE_REGISTRY.ByID[ccu.R.Collection.TypeID].Name
	}
	return ret
}

func collectionsTypedUids(collectionsContentUnits mdbmodels.CollectionsContentUnitSlice) []string {
	ret := make([]string, len(collectionsContentUnits))
	for i, ccu := range collectionsContentUnits {
		ret[i] = KeyValue(consts.ES_UID_TYPE_COLLECTION, ccu.R.Collection.UID)
	}
	return ret
}

func (index *ContentUnitsIndex) prepareIndexUnit(cu *mdbmodels.ContentUnit, indexData *IndexData) (map[string]Result, *IndexErrors) {
	indexErrors := MakeIndexErrors()
	// Create documents in each language with available translation
	i18nMap := make(map[string]Result)
	for _, i18n := range cu.R.ContentUnitI18ns {
		if i18n.Name.Valid && strings.TrimSpace(i18n.Name.String) != "" {
			typedUids := append([]string{KeyValue(consts.ES_UID_TYPE_CONTENT_UNIT, cu.UID)},
				collectionsTypedUids(cu.R.CollectionsContentUnits)...)
			filterValues := append([]string{KeyValue("content_type", mdb.CONTENT_TYPE_REGISTRY.ByID[cu.TypeID].Name)},
				KeyValues("collections_content_type", collectionsContentTypes(cu.R.CollectionsContentUnits))...)

			unit := Result{
				ResultType:   index.resultType,
				IndexDate:    &utils.Date{Time: time.Now()},
				MDB_UID:      cu.UID,
				TypedUids:    typedUids,
				FilterValues: filterValues,
				Title:        i18n.Name.String,
			}

			if i18n.Description.Valid && i18n.Description.String != "" {
				unit.Description = i18n.Description.String
			}

			if cu.Properties.Valid {
				var props map[string]interface{}
				err := json.Unmarshal(cu.Properties.JSON, &props)
				indexErrors.DocumentError(i18n.Language, err, fmt.Sprintf("json.Unmarshal properties %s", cu.UID))
				if err != nil {
					continue
				}
				if filmDate, ok := props["film_date"]; ok {
					dateStr := strings.Split(filmDate.(string), "T")[0] // remove the 'time' part
					val, err := time.Parse("2006-01-02", dateStr)
					indexErrors.DocumentError(i18n.Language, err, fmt.Sprintf("time.Parse film_date %s", cu.UID))
					if err != nil {
						continue
					}
					unit.EffectiveDate = &utils.Date{Time: val}
				}
			}

			if val, ok := indexData.Sources[cu.UID]; ok {
				unit.FilterValues = append(unit.FilterValues, KeyValues(consts.ES_UID_TYPE_SOURCE, val)...)
				unit.TypedUids = append(unit.TypedUids, KeyValues(consts.ES_UID_TYPE_SOURCE, val)...)
				//  We dont add TitleSuggest to CU with source
				unit.TitleSuggest = SuggestField{[]string{}, float64(0)}
			} else {
				unit.TitleSuggest = SuggestField{Suffixes(i18n.Name.String), float64(1)}
			}
			if val, ok := indexData.Tags[cu.UID]; ok {
				unit.FilterValues = append(unit.FilterValues, KeyValues(consts.ES_UID_TYPE_TAG, val)...)
				unit.TypedUids = append(unit.TypedUids, KeyValues(consts.ES_UID_TYPE_TAG, val)...)
			}
			if val, ok := indexData.MediaLanguages[cu.UID]; ok {
				unit.FilterValues = append(unit.FilterValues, KeyValues(consts.FILTER_LANGUAGE, val)...)
			}
			if byLang, ok := indexData.Transcripts[cu.UID]; ok {
				if val, ok := byLang[i18n.Language]; ok {
					var err error
					unit.Content, err = index.assetsService.Doc2Text(val[0])
					if unit.Content == "" {
						log.Warnf("Content Units Index - Transcript empty: %s", val[0])
					}
					indexErrors.DocumentError(i18n.Language, err, fmt.Sprintf("Content Units Index - Error parsing docx: %s", val[0]))
					if err == nil {
						unit.TypedUids = append(unit.TypedUids, KeyValue(consts.ES_UID_TYPE_FILE, val[0]))
					}
				}
			}

			i18nMap[i18n.Language] = unit
		}
	}

	return i18nMap, indexErrors
}
