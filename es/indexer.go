package es

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

type Indexer struct {
	indices []Index
}

func MakeProdIndexer(date string, mdb *sql.DB, esc *elastic.Client) (*Indexer, error) {
	return MakeIndexer("prod", date, consts.ES_ALL_RESULT_TYPES, mdb, esc)
}

func MakeFakeIndexer(mdb *sql.DB, esc *elastic.Client) (*Indexer, error) {
	return MakeIndexer("fake", "fake-date", []string{}, mdb, esc)
}

// Receives namespace and list of indexes names.
func MakeIndexer(namespace string, date string, names []string, mdb *sql.DB, esc *elastic.Client) (*Indexer, error) {
	log.Infof("Indexer - Make indexer - %s - %s - %s", namespace, date, strings.Join(names, ", "))
	indexer := new(Indexer)
	indexer.indices = make([]Index, len(names))
	for i, name := range names {
		if name == consts.ES_RESULT_TYPE_UNITS {
			indexer.indices[i] = MakeContentUnitsIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_SOURCES {
			indexer.indices[i] = MakeSourcesIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_TAGS {
			indexer.indices[i] = MakeTagsIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_COLLECTIONS {
			indexer.indices[i] = MakeCollectionsIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_BLOG_POSTS {
			indexer.indices[i] = MakeBlogIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_TWEETS {
			indexer.indices[i] = MakeTweeterIndex(namespace, date, mdb, esc)
		} else if name == consts.ES_RESULT_TYPE_LIKUTIM {
			indexer.indices[i] = MakeLikutimIndex(namespace, date, mdb, esc)
		} else {
			return nil, errors.New(fmt.Sprintf("MakeIndexer - Invalid index name: %+v", name))
		}
	}
	return indexer, nil
}

func ProdIndexDate(esc *elastic.Client) (error, string) {
	indexDate := viper.GetString("elasticsearch.index-date")
	if indexDate == "" {
		return aliasedIndexDate(esc, "prod", consts.ES_RESULTS_INDEX)
	}
	return nil, indexDate
}

// Deprecated, use ProdIndexDateForEvents, we should alway
//func ProdAliasedIndexDate(esc *elastic.Client) (error, string) {
//	return aliasedIndexDate(esc, "prod", consts.ES_RESULTS_INDEX)
//}

func aliasedIndexDate(esc *elastic.Client, namespace string, name string) (error, string) {
	aliasRegexp := IndexName(namespace, name, ".*", ".*")
	alias := indexAliasName(namespace, name, "%s")
	return AliasedIndex(esc, alias, aliasRegexp)
}

func AliasedIndex(esc *elastic.Client, alias string, aliasRegexp string) (error, string) {
	aliasesService := elastic.NewAliasesService(esc)
	prevIndicesByAlias := make(map[string]string)
	aliasesRes, err := aliasesService.Do(context.TODO())
	if err != nil {
		return errors.Wrapf(err, "Error fetching asiases, alias: %s regexp: %s", alias, aliasRegexp), ""
	}
	for indexName, indexResult := range aliasesRes.Indices {
		matched, err := regexp.MatchString(aliasRegexp, indexName)
		if err != nil {
			return errors.Wrapf(err, "Error matching regex, alias: %s, regexp: %s, indexName: %s", alias, aliasRegexp, indexName), ""
		}
		if matched {
			if len(indexResult.Aliases) > 1 {
				return errors.New(fmt.Sprintf("Expected no more then one alias for %s, got %d", indexName, len(indexResult.Aliases))), ""
			}
			if len(indexResult.Aliases) == 1 {
				prevIndicesByAlias[indexResult.Aliases[0].AliasName] = indexName
			}
		}
	}

	date := ""
	indicesExist := false
	for _, lang := range consts.ALL_KNOWN_LANGS {
		prevIndex, ok := prevIndicesByAlias[fmt.Sprintf(alias, lang)]
		if ok {
			indicesExist = true
			parts := strings.Split(prevIndex, "_")
			if len(parts) != 4 {
				return errors.New(fmt.Sprintf("Expected 4 parts in index name %s, got %d.", prevIndex, len(parts))), ""
			}
			if date == "" {
				date = parts[len(parts)-1]
			}
			if date != parts[len(parts)-1] {
				return errors.New(fmt.Sprintf("Expected index date to be %s got %s at index %s", date, parts[len(parts)], prevIndex)), ""
			}
		} else {
			if indicesExist {
				log.Warnf("Indexer - Did not find index name for %s", alias)
			}
		}
	}

	if date == "" && indicesExist {
		return errors.New("At least one aliased index should have date specified."), ""
	}

	return nil, date
}

func SwitchProdAliasToCurrentIndex(date string, esc *elastic.Client) error {
	return SwitchAliasToCurrentIndex("prod", consts.ES_RESULTS_INDEX, date, esc)
}

func SwitchAliasToCurrentIndex(namespace string, name string, date string, esc *elastic.Client) error {
	alias := indexAliasName(namespace, name, "%s")
	iName := IndexName(namespace, name, "%s", date)
	err, prevDate := aliasedIndexDate(esc, namespace, name)
	if err != nil {
		return errors.Wrapf(err, "Error getting prev date, namespace: %s, name: %s, date: %s.", namespace, name, date)
	}
	prevIndex := ""
	if prevDate != "" {
		prevIndex = IndexName(namespace, name, "%s", prevDate)
	}
	return SwitchAlias(alias, prevIndex, iName, esc)
}

func SwitchAlias(alias string, prev string, next string, esc *elastic.Client) error {
	finalErr := error(nil)
	for _, lang := range consts.ALL_KNOWN_LANGS {
		aliasService := elastic.NewAliasService(esc)
		if prev != "" {
			aliasService = aliasService.Remove(fmt.Sprintf(prev, lang), fmt.Sprintf(alias, lang))
		}
		aliasService.Add(fmt.Sprintf(next, lang), fmt.Sprintf(alias, lang))
		res, err := aliasService.Do(context.TODO())
		if err != nil || !res.Acknowledged {
			finalErr = utils.JoinErrors(err, errors.Wrapf(err, "Failed due to error or Acknowledged is false. prev: %s, next: %s, alias: %s.",
				fmt.Sprintf(prev, lang), fmt.Sprintf(next, lang), fmt.Sprintf(alias, lang)))
			continue
		}
	}
	return finalErr
}

func openIndex(indexName string, esc *elastic.Client) {
	openRes, err := esc.OpenIndex(indexName).Do(context.TODO())
	if err != nil {
		log.Error(errors.Wrapf(err, "OpenIndex: %s", indexName))
		return
	}
	if !openRes.Acknowledged {
		log.Errorf("OpenIndex not Acknowledged: %s", indexName)
		return
	}
	err = esc.WaitForYellowStatus("10s")
	if err != nil {
		log.Errorf("OpenIndex, failed waiting for yellow status.")
	}
}

type IndexNameByLang func(lang string) string

func IndexNameFuncByNamespaceAndDate(namespace string, indexDate string) IndexNameByLang {
	return func(lang string) string {
		if indexDate != "" {
			// Use specific date index.
			return IndexName(namespace, consts.ES_RESULTS_INDEX, lang, indexDate)
		} else {
			// Use prooduction (alias) index.
			return IndexNameForServing(namespace, consts.ES_RESULTS_INDEX, lang)
		}
	}
}

func UpdateSynonyms(esc *elastic.Client, indexNameByLang IndexNameByLang) error {
	folder, err := SynonymsFolder()
	if err != nil {
		return errors.Wrap(err, "SynonymsFolder not available.")
	}

	files, err := ioutil.ReadDir(folder)
	if err != nil {
		return errors.Wrap(err, "Cannot read synonym files list.")
	}

	type SynonymGraphSU struct {
		Type      string   `json:"type"`
		Tokenizer string   `json:"tokenizer"`
		Synonyms  []string `json:"synonyms"`
	}
	type FilterSU struct {
		SynonymGraph SynonymGraphSU `json:"synonym_graph"`
	}
	type AnalysisSU struct {
		Filter FilterSU `json:"filter"`
	}
	type IndexSU struct {
		Analysis AnalysisSU `json:"analysis"`
	}
	type SU struct {
		Index IndexSU `json:"index"`
	}
	body := SU{
		IndexSU{
			AnalysisSU{
				FilterSU{
					SynonymGraphSU{
						Type:      "synonym_graph",
						Tokenizer: "keyword",
					},
				},
			},
		},
	}

	for _, fileInfo := range files {
		keywords := make([]string, 0)

		//  Convention: file name without extension is the language code.
		var ext = filepath.Ext(fileInfo.Name())
		var lang = fileInfo.Name()[0 : len(fileInfo.Name())-len(ext)]
		if !utils.Contains(utils.Is(consts.ALL_KNOWN_LANGS), lang) {
			log.Warningf("Strange synonyms file: %s, skipping.", fileInfo.Name())
			continue
		}

		filePath := filepath.Join(folder, fileInfo.Name())
		file, err := os.Open(filePath)
		if err != nil {
			return errors.Wrapf(err, "Unable to open synonyms file: %s.", filePath)
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()

			//  Blank lines and lines starting with pound are comments (like Solr format).
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				if !strings.HasPrefix(trimmed, "#") {
					commaSeperated := strings.Replace(trimmed, "\t", ",", -1)
					keywords = append(keywords, commaSeperated)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return errors.Wrapf(err, "Error at scanning synonym config file: %s.", filePath)
		}
		// Set keywords to update synonyms
		body.Index.Analysis.Filter.SynonymGraph.Synonyms = keywords

		// Close the index in order to update the synonyms
		log.Infof("Synonyms language: %s.", lang)
		indexName := indexNameByLang(lang)
		closeRes, err := esc.CloseIndex(indexName).Do(context.TODO())
		if err != nil {
			return errors.Wrapf(err, "CloseIndex: %s", indexName)
		}
		if !closeRes.Acknowledged {
			return errors.New(fmt.Sprintf("CloseIndex not Acknowledged: %s", indexName))
		}

		defer openIndex(indexName, esc)

		bodyStr, err := json.Marshal(body)
		if err != nil {
			return errors.New(fmt.Sprintf("Failed marshding %+v.", body))
		}
		//  Using standard HTTP put call instead of esc.IndexPutSettings(indexName).BodyJson(body).Do(context.TODO())
		//	 due to the fact that this version of elastic hides error when synonyms are not updated properly.
		if runtime.GOOS == "windows" {
			indexName = url.QueryEscape(indexName)
		}
		url := fmt.Sprintf("%s/%s/_settings", viper.GetString("elasticsearch.url"), indexName)
		log.Infof("Sending to %s: %s", url, string(bodyStr))
		contents, err := putRequest(url, bytes.NewBuffer(bodyStr))
		if err != nil {
			return errors.Wrapf(err, "IndexPutSettings: %s with keywords: \n%s\n", indexName, strings.Join(keywords, "\n"))
		}
		settingsRes := new(elastic.IndicesPutSettingsResponse)
		if err := json.Unmarshal(contents, settingsRes); err != nil {
			return errors.New(fmt.Sprintf("Error decoding ret"))
		}
		if !settingsRes.Acknowledged {
			return errors.New(fmt.Sprintf("IndexPutSettings not Acknowledged: %s", indexName))
		}
	}
	return nil
}

func putRequest(url string, data io.Reader) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPut, url, data)
	req.Header.Set("Content-Type", "application/json")
	if err != nil {
		log.Fatal(err)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrapf(err, "While calling PUT request %s", url)
	}
	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Wrapf(err, "While reading PUT response for %s", url)
	}
	if response.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Error, received: %s", string(contents)))
	}
	return contents, nil
}

func (indexer *Indexer) ReindexAll(esc *elastic.Client) error {
	log.Info("Indexer - Re-Indexing everything")
	if err := indexer.CreateIndexes(); err != nil {
		return errors.Wrapf(err, "Error creating indexes.")
	}
	log.Info("Indexer - Updating Synonyms.")
	if len(indexer.indices) == 0 {
		return errors.New("Expected indices to be more than 0.")
	}
	if err := UpdateSynonyms(esc, IndexNameFuncByNamespaceAndDate(indexer.indices[0].Namespace(), indexer.indices[0].IndexDate())); err != nil {
		return errors.Wrapf(err, "Error updating synonyms.")
	}
	log.Info("Indexer - Synonymns updated.")
	done := make(chan string)
	errs := make([]error, len(indexer.indices))
	for i := range indexer.indices {
		go func(i int) {
			errs[i] = indexer.indices[i].ReindexAll()
			done <- indexer.indices[i].ResultType()
		}(i)
	}
	for _ = range indexer.indices {
		name := <-done
		log.Infof("Finished: %s", name)
	}
	err := (error)(nil)
	for i, e := range errs {
		err = utils.JoinErrorsWrap(err, e, fmt.Sprintf("Reindex of: %s", indexer.indices[i].IndexName("xx")))
	}
	return err
}

func (indexer *Indexer) RefreshAll() error {
	log.Info("Indexer - Refresh (sync new indexed documents) all indices.")
	err := (error)(nil)
	for _, index := range indexer.indices {
		err = utils.JoinErrorsWrap(err, index.RefreshIndex(), fmt.Sprintf("Error creating index: %s.", index.IndexName("xx")))
	}
	return err
}

func (indexer *Indexer) CreateIndexes() error {
	log.Infof("Indexer - Create new indices in elastic: %+v", indexer.indices)
	err := (error)(nil)
	for _, index := range indexer.indices {
		err = utils.JoinErrorsWrap(err, index.CreateIndex(), fmt.Sprintf("Error creating index: %s.", index.IndexName("xx")))
	}
	return err
}

func (indexer *Indexer) DeleteIndexes() error {
	log.Info("Indexer - Delete indices from elastic.")
	err := (error)(nil)
	for _, index := range indexer.indices {
		err = utils.JoinErrorsWrap(err, index.DeleteIndex(), fmt.Sprintf("Error creating index: %s.", index.IndexName("xx")))
	}
	return err
}

func (indexer *Indexer) Update(scope Scope) error {
	// Maps const.ES_UID_TYPE_* to list of uids.
	// This is required to correctly add the removed uids per type.
	removedByResultType := make(map[string][]string)
	err := (error)(nil)
	for _, index := range indexer.indices {
		removed, e := index.RemoveFromIndex(scope)
		for resultType, removedUIDs := range removed {
			if _, ok := removedByResultType[resultType]; !ok {
				removedByResultType[resultType] = []string{}
			}
			removedByResultType[resultType] = append(removedByResultType[resultType], removedUIDs...)
		}
		err = utils.JoinErrorsWrap(err, e, fmt.Sprintf("Error updating: %+v", scope))
	}
	err = utils.JoinErrorsWrap(err, indexer.RefreshAll(), fmt.Sprintf("Error Refreshing: %+v", scope))
	for _, index := range indexer.indices {
		removed, ok := removedByResultType[index.ResultType()]
		if !ok {
			removed = []string{}
		}
		err = utils.JoinErrorsWrap(err, index.AddToIndex(scope, removed), fmt.Sprintf("Error updating: %+v", scope))
	}
	return err
}

// Set of MDB event handlers to incrementally change all indexes.
func (indexer *Indexer) CollectionUpdate(uid string) error {
	log.Infof("Indexer - Index collection upadate event: %s", uid)
	return indexer.Update(Scope{CollectionUID: uid})
}

func (indexer *Indexer) ContentUnitUpdate(uid string) error {
	log.Infof("Indexer - Index content unit update  event: %s", uid)
	return indexer.Update(Scope{ContentUnitUID: uid})
}

func (indexer *Indexer) FileUpdate(uid string) error {
	log.Infof("Indexer - Index file update event: %s", uid)
	return indexer.Update(Scope{FileUID: uid})
}

func (indexer *Indexer) SourceUpdate(uid string) error {
	log.Infof("Indexer - Index source update event: %s", uid)
	return indexer.Update(Scope{SourceUID: uid})
}

func (indexer *Indexer) TagUpdate(uid string) error {
	log.Infof("Indexer - Index tag update  event: %s", uid)
	return indexer.Update(Scope{TagUID: uid})
}

func (indexer *Indexer) PersonUpdate(uid string) error {
	log.Infof("Indexer - Index person update  event: %s", uid)
	return indexer.Update(Scope{PersonUID: uid})
}

func (indexer *Indexer) PublisherUpdate(uid string) error {
	log.Infof("Indexer - Index publisher update event: %s", uid)
	return indexer.Update(Scope{PublisherUID: uid})
}

func (indexer *Indexer) BlogPostUpdate(id string) error {
	log.Infof("Indexer - Index blog post update event: %v", id)
	return indexer.Update(Scope{BlogPostWPID: id})
}

func (indexer *Indexer) TweetUpdate(tid string) error {
	log.Infof("Indexer - Index tweet update event: %v", tid)
	return indexer.Update(Scope{TweetTID: tid})
}
