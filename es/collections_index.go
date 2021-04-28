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
	"github.com/Bnei-Baruch/archive-backend/mdb"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

func MakeCollectionsIndex(namespace string, indexDate string, db *sql.DB, esc *elastic.Client) *CollectionsIndex {
	ci := new(CollectionsIndex)
	ci.resultType = consts.ES_RESULT_TYPE_COLLECTIONS
	ci.baseName = consts.ES_RESULTS_INDEX
	ci.namespace = namespace
	ci.db = db
	ci.esc = esc
	ci.indexDate = indexDate
	return ci
}

type CollectionsIndex struct {
	BaseIndex
}

func defaultCollectionsSql() string {
	return fmt.Sprintf("c.secure = 0 AND c.published IS TRUE AND c.type_id NOT IN (%d, %d, %d, %d, %d, %d, %d)",
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_DAILY_LESSON].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SPECIAL_LESSON].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_CLIPS].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SONGS].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BOOKS].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_HOLIDAY].ID, // we use grammar for holiday collections search
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_UNKNOWN].ID,
	)
}

func (index *CollectionsIndex) ReindexAll() error {
	log.Info("Collections Index - Reindex all.")
	_, indexErrors := index.RemoveFromIndexQuery(index.FilterByResultTypeQuery(index.resultType))
	if err := indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "CollectionsIndex"); err != nil {
		return err
	}
	return indexErrors.Join(index.addToIndexSql(defaultCollectionsSql()), "").CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "CollectionsIndex")
}

func (index *CollectionsIndex) RemoveFromIndex(scope Scope) (map[string][]string, error) {
	log.Debugf("CollectionsIndex - RemoveFromIndex. Scope: %+v.", scope)
	removed, indexErrors := index.removeFromIndex(scope)
	return removed, indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "CollectionsIndex")
}

func (index *CollectionsIndex) AddToIndex(scope Scope, removedUIDs []string) error {
	log.Debugf("CollectionsIndex - AddToIndex. Scope: %+v, revedUIDs: %+v.", scope, removedUIDs)
	return index.addToIndex(scope, removedUIDs).CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "CollectionsIndex")
}

func (index *CollectionsIndex) addToIndex(scope Scope, removedUIDs []string) *IndexErrors {
	sqlScope := defaultCollectionsSql()
	uids := removedUIDs
	if scope.CollectionUID != "" {
		uids = append(uids, scope.CollectionUID)
	}
	indexErrors := MakeIndexErrors()
	if scope.ContentUnitUID != "" {
		moreUIDs, err := CollectionsScopeByContentUnit(index.db, scope.ContentUnitUID)
		if indexErrors.SetError(err).Wrap(fmt.Sprintf("CollectionsIndex addToIndex scope.ContentUnitUID: %+v", scope.ContentUnitUID)).Error != nil {
			uids = append(uids, moreUIDs...)
		}
	}
	if len(uids) == 0 {
		return indexErrors
	}
	quoted := make([]string, len(uids))
	for i, uid := range uids {
		quoted[i] = fmt.Sprintf("'%s'", uid)
	}
	sqlScope = fmt.Sprintf("%s AND c.uid IN (%s)", sqlScope, strings.Join(quoted, ","))
	return indexErrors.Join(index.addToIndexSql(sqlScope), "collections index addToIndex addToIndexSql")
}

func (index *CollectionsIndex) removeFromIndex(scope Scope) (map[string][]string, *IndexErrors) {
	typedUIDs := make([]string, 0)
	if scope.CollectionUID != "" {
		typedUIDs = append(typedUIDs, KeyValue(consts.ES_UID_TYPE_COLLECTION, scope.CollectionUID))
	}
	if scope.FileUID != "" {
		typedUIDs = append(typedUIDs, KeyValue(consts.ES_UID_TYPE_FILE, scope.FileUID))
	}
	indexErrors := MakeIndexErrors()
	if scope.ContentUnitUID != "" {
		typedUIDs = append(typedUIDs, KeyValue(consts.ES_UID_TYPE_CONTENT_UNIT, scope.ContentUnitUID))
		moreUIDs, err := CollectionsScopeByContentUnit(index.db, scope.ContentUnitUID)
		indexErrors.SetError(err)
		typedUIDs = append(typedUIDs, KeyValues(consts.ES_UID_TYPE_COLLECTION, moreUIDs)...)
	}
	if scope.TagUID != "" {
		typedUIDs = append(typedUIDs, KeyValue(consts.ES_UID_TYPE_TAG, scope.TagUID))
	}
	if scope.SourceUID != "" {
		typedUIDs = append(typedUIDs, KeyValue(consts.ES_UID_TYPE_SOURCE, scope.SourceUID))
	}
	if len(typedUIDs) > 0 {
		typedUIDsI := make([]interface{}, len(typedUIDs))
		for i, typedUID := range typedUIDs {
			typedUIDsI[i] = typedUID
		}
		elasticScope := index.FilterByResultTypeQuery(index.resultType).
			Filter(elastic.NewTermsQuery("typed_uids", typedUIDsI...))
		uids, removeIndexErrors := index.RemoveFromIndexQuery(elasticScope)
		return uids, indexErrors.Join(removeIndexErrors, "CollectionsIndex, removeFromIndex")
	} else {
		// Nothing to remove.
		return make(map[string][]string), indexErrors
	}
}

func (index *CollectionsIndex) addToIndexSql(sqlScope string) *IndexErrors {
	var count int64
	if err := mdbmodels.NewQuery(index.db,
		qm.Select("count(*)"),
		qm.From("collections as c"),
		qm.Where(sqlScope)).QueryRow().Scan(&count); err != nil {
		return MakeIndexErrors().SetError(err)
	}
	log.Infof("Collections Index - Adding %d collections. Scope: %s.", count, sqlScope)
	offset := 0
	limit := 10
	totalIndexErrors := MakeIndexErrors()
	for offset < int(count) {
		var collections []*mdbmodels.Collection
		if err := mdbmodels.NewQuery(index.db,
			qm.From("collections as c"),
			qm.Load("CollectionI18ns"),
			qm.Load("CollectionsContentUnits"),
			qm.Load("CollectionsContentUnits.ContentUnit"),
			qm.Where(sqlScope),
			qm.Offset(offset),
			qm.Limit(limit)).Bind(&collections); err != nil {
			return totalIndexErrors.SetError(err).Wrap(fmt.Sprintf("Fetch collections from mdb. Offset: %d", offset))
		}
		log.Debugf("Adding %d collections (offset %d).", len(collections), offset)

		cuUIDs := make([]string, 0)
		for _, c := range collections {
			for _, ccu := range c.R.CollectionsContentUnits {
				cuUIDs = append(cuUIDs, fmt.Sprintf("'%s'", ccu.R.ContentUnit.UID))
			}
		}
		contentUnitsSqlScope := defaultContentUnitSql()
		if len(cuUIDs) > 0 {
			contentUnitsSqlScope = fmt.Sprintf(
				"%s AND cu.uid in (%s)", contentUnitsSqlScope, strings.Join(cuUIDs, ","))
		}

		indexErrors := MakeIndexErrors()
		for _, collection := range collections {
			indexErrors.Join(index.indexCollection(collection), "")
		}
		indexErrors.PrintIndexCounts(fmt.Sprintf("CollectionsIndex %d - %d", offset, offset+limit))
		offset += limit
		totalIndexErrors.Join(indexErrors, "")
	}
	return totalIndexErrors
}

func contentUnitsContentTypes(collectionsContentUnits mdbmodels.CollectionsContentUnitSlice) []string {
	m := make(map[string]bool)
	for _, ccu := range collectionsContentUnits {
		if defaultContentUnit(ccu.R.ContentUnit) {
			m[mdb.CONTENT_TYPE_REGISTRY.ByID[ccu.R.ContentUnit.TypeID].Name] = true
		}
	}
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func contentUnitsTypedUIDs(collectionsContentUnits mdbmodels.CollectionsContentUnitSlice) []string {
	ret := make([]string, len(collectionsContentUnits))
	for i, ccu := range collectionsContentUnits {
		ret[i] = KeyValue(consts.ES_UID_TYPE_CONTENT_UNIT, ccu.R.ContentUnit.UID)
	}
	return ret
}

func collectionSource(c *mdbmodels.Collection) string {
	if !c.Properties.Valid {
		return ""
	}
	var props map[string]interface{}
	err := json.Unmarshal(c.Properties.JSON, &props)
	if err != nil {
		return ""
	}
	if s, ok := props[consts.ES_UID_TYPE_SOURCE]; ok {
		return s.(string)
	}
	return ""
}

func (index *CollectionsIndex) indexCollection(c *mdbmodels.Collection) *IndexErrors {
	indexErrors := MakeIndexErrors()
	// Calculate effective date by choosing the last data of any of it's content units.
	effectiveDate := (*utils.Date)(nil)
	for _, ccu := range c.R.CollectionsContentUnits {
		cu := ccu.R.ContentUnit
		if cu.Properties.Valid {
			var props map[string]interface{}
			err := json.Unmarshal(cu.Properties.JSON, &props)
			indexErrors.DocumentError("", err, fmt.Sprintf("json.Unmarshal properties %s", cu.UID))
			if err != nil {
				continue
			}
			if filmDate, ok := props["film_date"]; ok {
				dateStr := strings.Split(filmDate.(string), "T")[0] // remove the 'time' part
				val, err := time.Parse("2006-01-02", dateStr)
				indexErrors.DocumentError("", err, fmt.Sprintf("time.Parse film_date %s", cu.UID))
				if err != nil {
					continue
				}
				if effectiveDate == nil || effectiveDate.Time.Before(val) {
					effectiveDate = &utils.Date{Time: val}
				}
			}
		}
	}
	source := collectionSource(c)
	// Create documents in each language with available translation
	i18nMap := make(map[string]Result)
	for _, i18n := range c.R.CollectionI18ns {
		if i18n.Name.Valid && i18n.Name.String != "" {
			indexErrors.ShouldIndex(i18n.Language)
			typedUIDs := append([]string{KeyValue(consts.ES_UID_TYPE_COLLECTION, c.UID)},
				contentUnitsTypedUIDs(c.R.CollectionsContentUnits)...)
			typedUIDs = append([]string{
				KeyValue(consts.ES_UID_TYPE_COLLECTION, c.UID),
			},
				contentUnitsTypedUIDs(c.R.CollectionsContentUnits)...)

			if source != "" {
				typedUIDs = append(typedUIDs, KeyValue("source", source))
			}

			filterValues := append(
				[]string{KeyValue("collections_content_type", mdb.CONTENT_TYPE_REGISTRY.ByID[c.TypeID].Name)},
				KeyValues("content_type", contentUnitsContentTypes(c.R.CollectionsContentUnits))...,
			)
			collection := Result{
				ResultType:   index.resultType,
				IndexDate:    &utils.Date{Time: time.Now()},
				MDB_UID:      c.UID,
				TypedUids:    typedUIDs,
				FilterValues: filterValues,
				Title:        i18n.Name.String,
				TitleSuggest: SuggestField{Suffixes(i18n.Name.String), float64(consts.ES_COLLECTIONS_SUGGEST_DEFAULT_WEIGHT)},
			}
			if effectiveDate != nil {
				collection.EffectiveDate = effectiveDate
			}

			if i18n.Description.Valid && i18n.Description.String != "" {
				collection.Description = i18n.Description.String
			}

			i18nMap[i18n.Language] = collection
		}
	}

	// Index each document in its language index
	for k, v := range i18nMap {
		name := index.IndexName(k)
		vBytes, err := json.Marshal(v)
		indexErrors.DocumentError(k, err, "CollectionsIndex, Failed marshal")
		if err != nil {
			continue
		}
		log.Debugf("Collections Index - Add collection %s to index %s", string(vBytes), name)
		resp, err := index.esc.Index().
			Index(name).
			Type("result").
			BodyJson(v).
			Do(context.TODO())
		indexErrors.DocumentError(k, err, fmt.Sprintf("Index collection %s %s", name, c.UID))
		if err != nil {
			continue
		}
		errNotCreated := (error)(nil)
		if resp.Result != "created" {
			errNotCreated = errors.New(fmt.Sprintf("Not created: collection %s %s", name, c.UID))
		} else {
			indexErrors.Indexed(k)
		}
		indexErrors.DocumentError(k, errNotCreated, "CollectionsIndex")
	}

	return indexErrors
}
