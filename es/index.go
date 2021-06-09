package es

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

const (
	LANGUAGES_MAX_FAILURE      = 3
	DOCUMENT_MAX_FAILIRE_RATIO = 0.1
)

type Scope struct {
	ContentUnitUID string
	FileUID        string
	CollectionUID  string
	TagUID         string
	SourceUID      string
	PersonUID      string
	PublisherUID   string
	TweetTID       string
	BlogPostWPID   string
}

type Index interface {
	ReindexAll() error
	RemoveFromIndex(scope Scope) (map[string][]string, error)
	AddToIndex(scope Scope, removedUIDs []string) error
	CreateIndex() error
	DeleteIndex() error
	RefreshIndex() error
	ResultType() string
	IndexName(language string) string
	IndexDate() string
	Namespace() string
}

type BaseIndex struct {
	resultType string
	namespace  string
	baseName   string
	indexDate  string
	db         *sql.DB
	esc        *elastic.Client
}

type DocumentError struct {
	Error error
	Count int
}

type IndexErrors struct {
	Error           error
	LanguageErrors  map[string][]error
	DocumentsCount  map[string]int
	DocumentsErrors map[string][]DocumentError

	ShouldIndexCount map[string]int
	IndexedCount     map[string]int
}

func MakeIndexErrors() *IndexErrors {
	return &IndexErrors{
		Error:            (error)(nil),
		LanguageErrors:   make(map[string][]error),
		DocumentsCount:   make(map[string]int),
		DocumentsErrors:  make(map[string][]DocumentError),
		ShouldIndexCount: make(map[string]int),
		IndexedCount:     make(map[string]int),
	}
}

func (indexErrors *IndexErrors) ShouldIndex(lang string) *IndexErrors {
	count, ok := indexErrors.ShouldIndexCount[lang]
	if !ok {
		indexErrors.ShouldIndexCount[lang] = 1
	} else {
		indexErrors.ShouldIndexCount[lang] = count + 1
	}
	return indexErrors
}

func (indexErrors *IndexErrors) Indexed(lang string) *IndexErrors {
	count, ok := indexErrors.IndexedCount[lang]
	if !ok {
		indexErrors.IndexedCount[lang] = 1
	} else {
		indexErrors.IndexedCount[lang] = count + 1
	}
	return indexErrors
}

func (indexErrors *IndexErrors) SetError(err error) *IndexErrors {
	indexErrors.Error = utils.JoinErrors(indexErrors.Error, err)
	return indexErrors
}

func (indexErrors *IndexErrors) Wrap(info string) *IndexErrors {
	indexErrors.Error = errors.Wrap(indexErrors.Error, info)
	return indexErrors
}

func (indexErrors *IndexErrors) Join(other *IndexErrors, info string) *IndexErrors {
	indexErrors.Error = utils.JoinErrorsWrap(indexErrors.Error, other.Error, info)
	for lang, otherErrors := range other.LanguageErrors {
		if errors, ok := indexErrors.LanguageErrors[lang]; ok {
			indexErrors.LanguageErrors[lang] = append(errors, otherErrors...)
		} else {
			indexErrors.LanguageErrors[lang] = otherErrors
		}
	}
	for lang, otherCount := range other.DocumentsCount {
		if count, ok := indexErrors.DocumentsCount[lang]; ok {
			indexErrors.DocumentsCount[lang] = count + otherCount
		} else {
			indexErrors.DocumentsCount[lang] = otherCount
		}
	}
	for lang, otherErrors := range other.DocumentsErrors {
		if errors, ok := indexErrors.DocumentsErrors[lang]; ok {
			indexErrors.DocumentsErrors[lang] = append(errors, otherErrors...)
		} else {
			indexErrors.DocumentsErrors[lang] = otherErrors
		}
	}
	for lang, otherShouldCount := range other.ShouldIndexCount {
		shouldCount, ok := indexErrors.ShouldIndexCount[lang]
		if !ok {
			indexErrors.ShouldIndexCount[lang] = otherShouldCount
		} else {
			indexErrors.ShouldIndexCount[lang] = shouldCount + otherShouldCount
		}
	}
	for lang, otherIndexedCount := range other.IndexedCount {
		indexedCount, ok := indexErrors.IndexedCount[lang]
		if !ok {
			indexErrors.IndexedCount[lang] = otherIndexedCount
		} else {
			indexErrors.IndexedCount[lang] = indexedCount + otherIndexedCount
		}
	}
	return indexErrors
}

func (indexErrors *IndexErrors) LanguageError(language string, e error, info string) *IndexErrors {
	if e != nil {
		indexErrors.LanguageErrors[language] = append(indexErrors.LanguageErrors[language], errors.Wrap(e, info))
	}
	return indexErrors
}

func (indexErrors *IndexErrors) DocumentErrorCount(language string, e error, info string, count int) *IndexErrors {
	if e != nil {
		indexErrors.DocumentsErrors[language] = append(indexErrors.DocumentsErrors[language], DocumentError{Error: errors.Wrap(e, info), Count: count})
	}
	if existingCount, ok := indexErrors.DocumentsCount[language]; ok {
		indexErrors.DocumentsCount[language] = existingCount
	} else {
		indexErrors.DocumentsCount[language] = existingCount + count
	}
	return indexErrors
}

func (indexErrors *IndexErrors) DocumentError(language string, e error, info string) *IndexErrors {
	return indexErrors.DocumentErrorCount(language, e, info, 1)
}

func (indexErrors *IndexErrors) PrintIndexCounts(index string) {
	counts := []string{}
	totalDidCount := 0
	totalShouldCount := 0
	for lang, should := range indexErrors.ShouldIndexCount {
		if should > 0 {
			did := indexErrors.IndexedCount[lang]
			counts = append(counts, fmt.Sprintf("%s: %d/%d", lang, did, should))
			totalDidCount += did
			totalShouldCount += should
		}
	}
	if len(counts) > 0 {
		log.Infof("Indexed [%s] %d/%d: %+v", index, totalDidCount, totalShouldCount, counts)
	} else {
		log.Infof("No indexing for [%s]", index)
	}
}

func (indexErrors *IndexErrors) PrintLanguageErrors(info string) {
	langs := []string{}
	langsErrors := []string{}
	for lang, errors := range indexErrors.LanguageErrors {
		if len(errors) > 0 {
			langs = append(langs, fmt.Sprintf("%s (%d)", lang, len(errors)))
			langsErrors = append(langsErrors, fmt.Sprintf("\t%s:", lang))
		}
		for _, err := range errors {
			langsErrors = append(langsErrors, fmt.Sprintf("\tError: %+v", err))
		}
	}
	if len(langs) > 0 {
		log.Infof("Language Errors [%s]: %+v", info, langs)
		fmt.Printf("Language Errors [%s]:\n%s", info, strings.Join(langsErrors, ","))
	} else {
		log.Infof("Language Errors [%s]: No Errors.", info)
	}
}

func (indexErrors *IndexErrors) PrintDocumentsErrors(info string) {
	langs := []string{}
	docsErrors := []string{}
	for lang, errors := range indexErrors.DocumentsErrors {
		if len(errors) > 0 {
			langs = append(langs, fmt.Sprintf("%s (%d/%d)", lang, len(errors), indexErrors.DocumentsCount[lang]))
			docsErrors = append(docsErrors, fmt.Sprintf("\t%s:", lang))
		}
		for _, documentError := range errors {
			docsErrors = append(docsErrors, fmt.Sprintf("\tError (%d): %+v", documentError.Count, documentError.Error))
		}
	}
	if len(langs) > 0 {
		log.Infof("Document Errors [%s]: %+v", info, langs)
		fmt.Printf("Document Errors [%s]:\n%s", info, strings.Join(docsErrors, ","))
	} else {
		log.Infof("Document Errors [%s]: No Errors.", info)
	}
}

func (indexErrors *IndexErrors) FailedLanguages() string {
	keys := []string{}
	for lang := range indexErrors.LanguageErrors {
		keys = append(keys, lang)
	}
	return strings.Join(keys, ",")
}

func (indexErrors *IndexErrors) LanguagesError() error {
	err := (error)(nil)
	for lang, languageErrors := range indexErrors.LanguageErrors {
		errorsStr := []string{}
		for _, err := range languageErrors {
			errorsStr = append(errorsStr, err.Error())
		}
		err = utils.JoinErrors(err, errors.New(fmt.Sprintf("%s: %s", lang, strings.Join(errorsStr, ". "))))
	}
	return err
}

func (indexErrors *IndexErrors) CheckErrors(languagesMaxFailure int, documentsMaxFailure float32, index string) error {
	err := indexErrors.Error
	if len(indexErrors.LanguageErrors) > languagesMaxFailure {
		err = utils.JoinErrors(err, errors.Wrapf(indexErrors.LanguagesError(), "[%s] Too many languages failed: %s.", index, indexErrors.FailedLanguages()))
		log.Errorf("[%s] Too many languages failed: %s.", index, indexErrors.FailedLanguages())
	}
	indexErrors.PrintLanguageErrors(index)
	for lang, documentErrors := range indexErrors.DocumentsErrors {
		count := indexErrors.DocumentsCount[lang]
		errCount := 0
		for _, de := range documentErrors {
			errCount = errCount + de.Count
		}
		if float32(errCount)/float32(count) > documentsMaxFailure {
			err = utils.JoinErrors(err, errors.New(fmt.Sprintf("[%s] Too many document errors, lang: %s - %d / %d", index, lang, len(documentErrors), count)))
			log.Errorf("[%s] Too many document errors, lang: %s - %d / %d", index, lang, len(documentErrors), count)
		}
		indexErrors.PrintDocumentsErrors(index)
	}
	indexErrors.PrintIndexCounts(index)
	return err
}

func indexAliasName(namespace string, name string, lang string) string {
	if namespace == "" || name == "" || lang == "" {
		panic(fmt.Sprintf("Not expecting empty parameter for IndexName, provided: (%s, %s, %s)", namespace, name, lang))
	}
	return fmt.Sprintf("%s_%s_%s", namespace, name, lang)
}

func IndexNameForServing(namespace string, name string, lang string) string {
	return indexNameByDefinedDateOrAlias(namespace, name, lang)
}

func IndexName(namespace string, name string, lang string, date string) string {
	if date == "" {
		panic(fmt.Sprintf("Not expecting empty parameter for IndexName, provided: (%s, %s, %s, %s)", namespace, name, lang, date))
	}
	return fmt.Sprintf("%s_%s", indexAliasName(namespace, name, lang), date)
}

func (index *BaseIndex) IndexDate() string {
	return index.indexDate
}

func (index *BaseIndex) Namespace() string {
	return index.namespace
}

func (index *BaseIndex) ResultType() string {
	return index.resultType
}

func (index *BaseIndex) IndexName(lang string) string {
	if index.namespace == "" || index.baseName == "" || index.indexDate == "" {
		panic("Index namespace, baseName and indexDate should be set.")
	}
	return IndexName(index.namespace, index.baseName, lang, index.indexDate)
}

func indexNameByDefinedDateOrAlias(namespace string, name string, lang string) string {
	indexDate := viper.GetString("elasticsearch.index-date")
	if indexDate == "" {
		return indexAliasName(namespace, name, lang)
	}
	log.Warnf("Using specific non prod index: %s", indexDate)
	return IndexName(namespace, name, lang, indexDate)
}

func (index *BaseIndex) indexAliasName(lang string) string {
	if index.namespace == "" || index.baseName == "" {
		panic("Index namespace and baseName should be set.")
	}
	return indexAliasName(index.namespace, index.baseName, lang)
}

func (index *BaseIndex) CreateIndex() error {
	for _, lang := range consts.ALL_KNOWN_LANGS {
		name := index.IndexName(lang)

		// Do nothing if index already exists.
		exists, err := index.esc.IndexExists(name).Do(context.TODO())
		log.Debugf("Create index, exists: %t.", exists)
		if err != nil {
			return errors.Wrapf(err, "Create index, lang: %s, name: %s.", lang, name)
		}
		if exists {
			log.Debugf("Index already exists (%+v), skipping.", name)
			continue
		}

		// Read mappings
		mappings, err := ReadDataFile(fmt.Sprintf("%s-%s.json", index.baseName, lang), "es", "mappings", index.baseName)
		if err != nil {
			return errors.Wrapf(err, "Read mapping file")
		}
		var bodyJson map[string]interface{}
		if err = json.Unmarshal(mappings, &bodyJson); err != nil {
			return errors.Wrap(err, "json.Unmarshal")
		}

		// Create index.
		res, err := index.esc.CreateIndex(name).BodyJson(bodyJson).Do(context.TODO())
		if err != nil {
			return errors.Wrap(err, "Create index")
		}
		if !res.Acknowledged {
			return errors.Errorf("Index creation wasn't acknowledged: %s", name)
		}
		log.Debugf("Created index: %+v", name)
	}
	return nil
}

func (index *BaseIndex) DeleteIndex() error {
	for _, lang := range consts.ALL_KNOWN_LANGS {
		if err := index.deleteIndexByLang(lang); err != nil {
			return err
		}
	}
	return nil
}

func (index *BaseIndex) deleteIndexByLang(lang string) error {
	i18nName := index.IndexName(lang)
	exists, err := index.esc.IndexExists(i18nName).Do(context.TODO())
	if err != nil {
		return err
	}
	if exists {
		res, err := index.esc.DeleteIndex(i18nName).Do(context.TODO())
		if err != nil {
			return errors.Wrap(err, "Delete index")
		}
		if !res.Acknowledged {
			return errors.Errorf("Index deletion wasn't acknowledged: %s", i18nName)
		}
	}
	return nil
}

func (index *BaseIndex) RefreshIndex() error {
	err := (error)(nil)
	for _, lang := range consts.ALL_KNOWN_LANGS {
		if err := index.RefreshIndexByLang(lang); err != nil {
			err = utils.JoinErrors(err, errors.Wrapf(err, "Error refreshing index for lang: %s", lang))
		}
	}
	return err
}

func (index *BaseIndex) RefreshIndexByLang(lang string) error {
	_, err := index.esc.Refresh(index.IndexName(lang)).Do(context.TODO())
	// fmt.Printf("\n\n\nShards: %+v \n\n\n", shards)
	return err
}

func (index *BaseIndex) FilterByResultTypeQuery(resultType string) *elastic.BoolQuery {
	return elastic.NewBoolQuery().Filter(elastic.NewTermsQuery(consts.ES_RESULT_TYPE, resultType))
}

type ScrollResult struct {
	MdbUid     string
	Id         string
	ResultType string
	ScrollId   string
}

func (index *BaseIndex) Scroll(indexName string, elasticScope elastic.Query) ([]ScrollResult, error) {
	var ret []ScrollResult
	var searchResult *elastic.SearchResult
	for true {
		if searchResult != nil && searchResult.Hits != nil {
			for _, h := range searchResult.Hits.Hits {
				result := Result{}
				json.Unmarshal(*h.Source, &result)
				ret = append(ret, ScrollResult{MdbUid: result.MDB_UID, Id: h.Id, ResultType: result.ResultType, ScrollId: searchResult.ScrollId})
			}
		}
		var err error
		scrollClient := index.esc.Scroll().Index(indexName).Query(elasticScope).Scroll("10m").Size(100)
		if searchResult != nil {
			scrollClient = scrollClient.ScrollId(searchResult.ScrollId)
		}
		searchResult, err = scrollClient.Do(context.TODO())
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	return ret, nil
}

func (index *BaseIndex) RemoveFromIndexQuery(elasticScope elastic.Query) (map[string][]string, *IndexErrors) {
	source, err := elasticScope.Source()
	indexErrors := MakeIndexErrors()
	if err != nil {
		return make(map[string][]string), indexErrors.SetError(errors.Wrapf(err, "Error getting scope query source for %s", index.resultType))
	}
	jsonBytes, err := json.Marshal(source)
	if err != nil {
		return make(map[string][]string), indexErrors.SetError(errors.Wrapf(err, "Error marshling scope query source for %s", index.resultType))
	}
	log.Infof("Results Index - Removing from index. Scope: %s", string(jsonBytes))
	removed := make(map[string]map[string]bool)
	totalShouldRemove := 0
	totalRemoved := 0
	for _, lang := range consts.ALL_KNOWN_LANGS {
		indexName := index.IndexName(lang)
		scrollResults, e := index.Scroll(indexName, elasticScope)
		if e != nil {
			indexErrors.LanguageError(lang, e, fmt.Sprintf("Error scrolling for deleting %s from index: %s", string(jsonBytes), indexName))
			continue
		}
		bulkService := elastic.NewBulkService(index.esc).Index(indexName)
		shouldRemoveCount := 0
		for _, scrollResult := range scrollResults {
			bulkService.Add(elastic.NewBulkDeleteRequest().Id(scrollResult.Id)).Type("result")
			shouldRemoveCount++
			if _, ok := removed[scrollResult.ResultType]; !ok {
				removed[scrollResult.ResultType] = make(map[string]bool)
			}
			index.esc.ClearScroll(scrollResult.ScrollId)
			removed[scrollResult.ResultType][scrollResult.MdbUid] = true
		}
		if shouldRemoveCount > 0 {
			totalShouldRemove = totalShouldRemove + shouldRemoveCount
			log.Infof("Should remove: %d from %s", shouldRemoveCount, indexName)
			bulkRes, e := bulkService.Do(context.TODO())
			if e != nil {
				indexErrors.LanguageError(lang, e, fmt.Sprintf("Results Index - Remove from index %s %+v.", indexName, elasticScope))
				continue
			}
			deletions := make(map[string]map[string]int)
			langRemoved := 0
			for _, itemMap := range bulkRes.Items {
				for _, res := range itemMap {
					if _, ok := deletions[res.Index]; !ok {
						deletions[res.Index] = make(map[string]int)
					}
					deletionsByIndex := deletions[res.Index]
					if _, ok := deletionsByIndex[res.Result]; !ok {
						deletionsByIndex[res.Result] = 0
					}
					deletionsByIndex[res.Result]++
					if res.Result == "deleted" {
						langRemoved++
					}
				}
			}
			for index, deletionsByIndex := range deletions {
				deletionsStrings := []string{}
				for result, count := range deletionsByIndex {
					deletionsStrings = append(deletionsStrings, fmt.Sprintf("%s: %d", result, count))
				}
				log.Infof("Deletions [%s] - %d: %+v", index, langRemoved, deletionsStrings)
			}
			totalRemoved = totalRemoved + langRemoved
		}
	}
	if len(removed) == 0 {
		log.Infof("Results Index - Nothing was delete.")
		return make(map[string][]string), indexErrors
	}
	log.Infof("Removed %d / %d", totalRemoved, totalShouldRemove)
	removedByType := make(map[string][]string)
	for resultType, removedByUID := range removed {
		for uid := range removedByUID {
			if _, ok := removedByType[resultType]; !ok {
				removedByType[resultType] = []string{}
			}
			removedByType[resultType] = append(removedByType[resultType], uid)
		}
	}
	return removedByType, indexErrors
}
