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

func MakeLikutimIndex(namespace string, indexDate string, db *sql.DB, esc *elastic.Client) *LikutimIndex {
	li := new(LikutimIndex)
	li.resultType = consts.ES_RESULT_TYPE_LIKUTIM
	li.baseName = consts.ES_RESULTS_INDEX
	li.namespace = namespace
	li.indexDate = indexDate
	li.db = db
	li.esc = esc
	return li
}

type LikutimIndex struct {
	BaseIndex
	Progress uint64
}

func (index *LikutimIndex) ReindexAll() error {
	log.Info("LikutimIndex.Reindex All.")
	_, indexErrors := index.RemoveFromIndexQuery(index.FilterByResultTypeQuery(index.resultType))
	if err := indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "LikutimIndex"); err != nil {
		return err
	}
	// SQL to always match any likutim
	sqlScope := fmt.Sprintf("secure = 0 AND published IS TRUE AND type_id = %d", mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LIKUTIM].ID)
	return indexErrors.Join(index.addToIndexSql(sqlScope), "").CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "LikutimIndex")
}

func (index *LikutimIndex) RemoveFromIndex(scope Scope) (map[string][]string, error) {
	log.Debugf("LikutimIndex.Update - Scope: %+v.", scope)
	removed, indexErrors := index.removeFromIndex(scope)
	return removed, indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "LikutimIndex")
}

func (index *LikutimIndex) AddToIndex(scope Scope, removedUIDs []string) error {
	log.Debugf("LikutimIndex.AddToIndex - Scope: %+v, removedUIDs: %+v.", scope, removedUIDs)
	return index.addToIndex(scope, removedUIDs).CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "LikutimIndex")
}

func (index *LikutimIndex) addToIndex(scope Scope, removedUIDs []string) *IndexErrors {
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
	if len(uids) == 0 {
		return indexErrors
	}
	quoted := make([]string, len(uids))
	for i, uid := range uids {
		quoted[i] = fmt.Sprintf("'%s'", uid)
	}
	sqlScope := fmt.Sprintf("secure = 0 AND published IS TRUE AND type_id = %d", mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LIKUTIM].ID)
	return indexErrors.Join(index.addToIndexSql(sqlScope), fmt.Sprintf("Failed adding to index: %+v", sqlScope))
}

func (index *LikutimIndex) addToIndexSql(sqlScope string) *IndexErrors {
	indexErrors := MakeIndexErrors()
	count, err := mdbmodels.ContentUnits(index.db, qm.Where(sqlScope)).Count()
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
		for offset < int(count) {
			tasks <- OffsetLimitJob{offset, limit, int(count)}
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

func (index *LikutimIndex) removeFromIndex(scope Scope) (map[string][]string, *IndexErrors) {
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
	if scope.TagUID != "" {
		typedUids = append(typedUids, KeyValue(consts.ES_UID_TYPE_TAG, scope.TagUID))
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

func (index *LikutimIndex) bulkIndexUnits(bulk OffsetLimitJob, sqlScope string) *IndexErrors {
	indexErrors := MakeIndexErrors()
	var units []*mdbmodels.ContentUnit
	if err := mdbmodels.NewQuery(index.db,
		qm.From("content_units as cu"),
		qm.Load("ContentUnitI18ns"),
		qm.Load("Tags"),
		qm.Where(sqlScope),
		qm.Offset(bulk.Offset),
		qm.Limit(bulk.Limit)).Bind(&units); err != nil {
		return indexErrors.SetError(errors.Wrap(err, "Fetch units from mdb"))
	}
	log.Infof("Content Units Index - Adding %d units (offset: %d total: %d).", len(units), bulk.Offset, bulk.Total)

	indexData, err := MakeIndexDataLikutim(index.db, sqlScope)
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
		for _, result := range results {
			indexErrors.ShouldIndex(lang)
			vBytes, err := json.Marshal(result)
			log.Debugf("Likutim Index - Add likutim %s to index %s", string(vBytes), indexName)
			resp, err := index.esc.Index().
				Index(indexName).
				Type("result").
				BodyJson(result).
				Do(context.TODO())
			indexErrors.DocumentError(lang, err, fmt.Sprintf("Index likutim %s %s", indexName, result.MDB_UID))
			if err != nil {
				return indexErrors
			}
			errNotCreated := (error)(nil)
			if resp.Result != "created" {
				errNotCreated = errors.New(fmt.Sprintf("Not created: tweet %s %s", indexName, result.MDB_UID))
			} else {
				indexErrors.Indexed(lang)
			}
			indexErrors.DocumentError(lang, errNotCreated, "TweeterIndex")
		}
	}
	indexErrors.PrintIndexCounts(fmt.Sprintf("ContentUnitIndex %d - %d / %d", bulk.Offset, bulk.Offset+bulk.Limit, bulk.Total))
	return indexErrors
}

func (index *LikutimIndex) prepareIndexUnit(cu *mdbmodels.ContentUnit, indexData *IndexData) (map[string]Result, *IndexErrors) {
	indexErrors := MakeIndexErrors()
	// Create documents in each language with available translation
	i18nMap := make(map[string]Result)
	files, err := mdbmodels.Files(index.db, qm.Where("secure = 0 AND published IS TRUE AND content_unit_id = ?", cu.ID)).All()
	if err != nil {
		indexErrors.SetError(err)
	}
	for _, i18n := range cu.R.ContentUnitI18ns {
		if i18n.Name.Valid && strings.TrimSpace(i18n.Name.String) != "" {
			content, err := index.getContent(files, i18n.Language)
			if err != nil {
				indexErrors.DocumentError(i18n.Language, err, fmt.Sprintf("LikutimIndex, unit uid: %s", cu.UID))
			}
			unit := Result{
				ResultType:   index.resultType,
				IndexDate:    &utils.Date{Time: time.Now()},
				MDB_UID:      cu.UID,
				TypedUids:    []string{KeyValue(consts.ES_UID_TYPE_LIKUTIM, cu.UID)},
				Title:        i18n.Name.String,
				Content:      content,
				TitleSuggest: SuggestField{[]string{}, float64(0)},
			}
			unit.FilterValues = append(unit.FilterValues, KeyValues(consts.FILTERS[consts.FILTER_UNITS_CONTENT_TYPES], []string{consts.CT_LIKUTIM})...)
			if val, ok := indexData.Tags[cu.UID]; ok {
				unit.FilterValues = append(unit.FilterValues, KeyValues(consts.ES_UID_TYPE_TAG, val)...)
				unit.TypedUids = append(unit.TypedUids, KeyValues(consts.ES_UID_TYPE_TAG, val)...)
			}

			i18nMap[i18n.Language] = unit
		}
	}

	return i18nMap, indexErrors
}
func (index *LikutimIndex) getContent(files []*mdbmodels.File, lang string) (string, error) {

	var file *mdbmodels.File
	for _, f := range files {
		if !f.Language.Valid || lang != f.Language.String {
			continue
		}
		ex := strings.Split(f.Name, ".")[1]
		if ex == "docx" {
			file = f
		}
		if file == nil && ex == "doc" {
			file = f
		}
	}
	if file == nil {
		return "", errors.New(fmt.Sprint("No .docx or .doc"))
	}

	return DocText(file.UID)
}
