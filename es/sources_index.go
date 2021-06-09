package es

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Bnei-Baruch/sqlboiler/queries"
	"github.com/Bnei-Baruch/sqlboiler/queries/qm"
	log "github.com/Sirupsen/logrus"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

func MakeSourcesIndex(namespace string, indexDate string, db *sql.DB, esc *elastic.Client) *SourcesIndex {
	si := new(SourcesIndex)
	si.resultType = consts.ES_RESULT_TYPE_SOURCES
	si.baseName = consts.ES_RESULTS_INDEX
	si.namespace = namespace
	si.indexDate = indexDate
	si.db = db
	si.esc = esc
	return si
}

type SourcesIndex struct {
	BaseIndex
	Progress uint64
}

func (index *SourcesIndex) ReindexAll() error {
	log.Info("SourcesIndex.Reindex All.")
	_, indexErrors := index.RemoveFromIndexQuery(index.FilterByResultTypeQuery(index.resultType))
	if err := indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "SourcesIndex"); err != nil {
		return err
	}
	// SQL to always match any source
	return indexErrors.Join(index.addToIndexSql("1=1"), "").CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "SourcesIndex")
}

func (index *SourcesIndex) RemoveFromIndex(scope Scope) (map[string][]string, error) {
	log.Debugf("SourcesIndex.Update - Scope: %+v.", scope)
	removed, indexErrors := index.removeFromIndex(scope)
	return removed, indexErrors.CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "SourcesIndex")
}

func (index *SourcesIndex) AddToIndex(scope Scope, removedUIDs []string) error {
	log.Debugf("SourcesIndex.AddToIndex - Scope: %+v, removedUIDs: %+v.", scope, removedUIDs)
	return index.addToIndex(scope, removedUIDs).CheckErrors(LANGUAGES_MAX_FAILURE, DOCUMENT_MAX_FAILIRE_RATIO, "SourcesIndex")
}

func (index *SourcesIndex) addToIndex(scope Scope, removedUIDs []string) *IndexErrors {
	uids := removedUIDs
	if scope.SourceUID != "" {
		uids = append(uids, scope.SourceUID)
	}
	if len(uids) == 0 {
		return MakeIndexErrors()
	}
	quoted := make([]string, len(uids))
	for i, uid := range uids {
		quoted[i] = fmt.Sprintf("'%s'", uid)
	}
	sqlScope := fmt.Sprintf("source.uid IN (%s)", strings.Join(quoted, ","))
	return index.addToIndexSql(sqlScope)
}

func (index *SourcesIndex) removeFromIndex(scope Scope) (map[string][]string, *IndexErrors) {
	if scope.SourceUID != "" {
		elasticScope := index.FilterByResultTypeQuery(index.resultType).
			Filter(elastic.NewTermsQuery("mdb_uid", scope.SourceUID))
		return index.RemoveFromIndexQuery(elasticScope)
	}

	// Nothing to remove.
	return make(map[string][]string), MakeIndexErrors()
}

func (index *SourcesIndex) bulkIndexSources(
	bulk OffsetLimitJob, sqlScope string,
	codesMap map[string][]string,
	idsMap map[string][]int64,
	authorsByLanguageMap map[string]map[string][]string) *IndexErrors {
	var sources []*mdbmodels.Source
	if err := mdbmodels.NewQuery(index.db,
		qm.From("sources as source"),
		qm.Load("SourceI18ns"),
		qm.Load("Authors"),
		qm.Where(sqlScope),
		qm.Offset(bulk.Offset),
		qm.Limit(bulk.Limit)).Bind(&sources); err != nil {
		return MakeIndexErrors().SetError(err).Wrap("SourcesIndex.addToIndexSql - Fetch sources from mdb.")
	}

	log.Infof("SourcesIndex.addToIndexSql - Adding %d sources (offset: %d total: %d).", len(sources), bulk.Offset, bulk.Total)

	indexErrors := MakeIndexErrors()
	for _, source := range sources {
		if parents, ok := codesMap[source.UID]; !ok {
			log.Warnf("SourcesIndex.addToIndexSql - Source %s not found in codesMap: %+v", source.UID, codesMap)
		} else if parentIds, ok := idsMap[source.UID]; !ok {
			log.Warnf("SourcesIndex.addToIndexSql - Source %s not found in idsMap: %+v", source.UID, idsMap)
		} else if authors, ok := authorsByLanguageMap[source.UID]; !ok {
			log.Warnf("SourcesIndex.addToIndexSql - Source %s not found in authorsByLanguageMap: %+v", source.UID, idsMap)
		} else {
			sourceIndexErrors := index.indexSource(source, parents, parentIds, authors)
			indexErrors.Join(sourceIndexErrors, fmt.Sprintf("SourcesIndex.addToIndexSql - Unable to index source '%s' (uid: %s).", source.Name, source.UID))
		}
	}
	indexErrors.PrintIndexCounts(fmt.Sprintf("SourcedIndex %d - %d", bulk.Offset, bulk.Offset+bulk.Limit))
	return indexErrors
}

// Note: scope usage is limited to source.uid only (e.g. source.uid='L2jMWyce')
func (index *SourcesIndex) addToIndexSql(sqlScope string) *IndexErrors {
	var count int
	if err := mdbmodels.NewQuery(index.db,
		qm.Select("COUNT(1)"),
		qm.From("sources as source"),
		qm.Where(sqlScope)).QueryRow().Scan(&count); err != nil {
		return MakeIndexErrors().SetError(err).Wrap("SourcesIndex, addToIndexSql")
	}

	log.Debugf("SourcesIndex.addToIndexSql - Sources Index - Adding %d sources. Scope: %s", count, sqlScope)

	// Loading all sources path in all languages.
	// codesMap from uid => codes
	// idsMap from uid => id
	// authorsByLanguageMap from uid => lang => authors
	codesMap, idsMap, authorsByLanguageMap, err := index.loadSources(index.db, sqlScope)
	if err != nil {
		return MakeIndexErrors().SetError(err).Wrap("SourcesIndex.addToIndexSql - Fetch sources parents from mdb.")
	}

	tasks := make(chan OffsetLimitJob, 300)
	errors := make(chan *IndexErrors, 300)
	doneAdding := make(chan bool)

	tasksCount := 0
	go func() {
		offset := 0
		limit := utils.MaxInt(10, utils.MinInt(100, (int)(count/10)))
		for offset < int(count) {
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
				errors <- index.bulkIndexSources(task, sqlScope, codesMap, idsMap, authorsByLanguageMap)
			}
		}(tasks, errors)
	}

	<-doneAdding
	indexErrors := MakeIndexErrors()
	for a := 1; a <= tasksCount; a++ {
		indexErrors.Join(<-errors, "")
	}
	return indexErrors
}

func (index *SourcesIndex) loadSources(db *sql.DB, sqlScope string) (map[string][]string, map[string][]int64, map[string]map[string][]string, error) {
	rows, err := queries.Raw(db, fmt.Sprintf(`
		WITH recursive rec_sources AS
        (
               SELECT s.id,
                      s.uid,
                      array [s.uid] "path",
                      array [s.id] :: bigint [] "idspath",
                      authors,
                      aun.language,
                      aun.author_names
               FROM   sources s
               JOIN
                      (
                               SELECT   source_id,
                                        array_agg(a.id),
                                        array_agg(a.code) AS authors
                               FROM     authors_sources aas
                               JOIN     authors a
                               ON       a.id = aas.author_id
                               GROUP BY source_id) au
               ON     au.source_id = s.id
               JOIN
                      (
                                SELECT    source_id,
                                          an.language,
										  array [an.name] AS author_names
										  -- We dont take the author full name (an.full_name) value and use synonyms instead                   
											/* CASE
													WHEN an.full_name IS NULL THEN array [an.name]
													ELSE array [an.name, an.full_name]
												END AS author_names */
                                FROM      authors_sources aas
                                JOIN      authors a
                                ON        a.id = aas.author_id
                                LEFT JOIN author_i18n an
                                ON        an.author_id=a.id ) aun
               ON     aun.source_id = s.id
               UNION
               SELECT     s.id,
                          s.uid,
                          (rs.path
                                     || s.uid) :: character(8)[],
                          (rs.idspath
                                     || s.id) :: bigint [],
                          rs.authors,
                          rs.language,
                          rs.author_names
               FROM       sources s
               INNER JOIN rec_sources rs
               ON         s.parent_id = rs.id )
        SELECT uid,
               array_cat(path, authors) "path",
               idspath,
               language,
               author_names
		FROM   rec_sources source 
		where %s;`, sqlScope)).Query()

	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "SourcesIndex.loadSources - Query failed.")
	}
	defer rows.Close()

	codeMap := make(map[string][]string)
	idMap := make(map[string][]int64)
	authorsByLanguageMap := make(map[string]map[string][]string)

	for rows.Next() {
		var uid string
		var lang string
		var authors pq.StringArray
		var codeValues pq.StringArray
		var idValues pq.Int64Array
		err := rows.Scan(&uid, &codeValues, &idValues, &lang, &authors)
		if err != nil {
			return nil, nil, nil, errors.Wrap(err, "rows.Scan")
		}
		if authorsByLanguageMap[uid] == nil {
			authorsByLanguageMap[uid] = make(map[string][]string)
		}
		stringAuthors := make([]string, 0)
		for _, ns := range authors {
			stringAuthors = append(stringAuthors, ns)
		}
		stringCodeValues := make([]string, 0)
		for _, ns := range codeValues {
			stringCodeValues = append(stringCodeValues, ns)
		}
		authorsByLanguageMap[uid][lang] = stringAuthors
		if _, ok := codeMap[uid]; !ok {
			codeMap[uid] = stringCodeValues
			idMap[uid] = idValues
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, errors.Wrap(err, "SourcesIndex.loadSources - rows.Err()")
	}

	return codeMap, idMap, authorsByLanguageMap, nil
}

func (index *SourcesIndex) getDocxPath(uid string, lang string) (string, error) {
	uidPath := path.Join(SourcesFolder(), uid)
	jsonPath := path.Join(uidPath, "index.json")
	jsonCnt, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return "", fmt.Errorf("SourcesIndex.getDocxPath - Unable to read from file %s. Error: %+v", jsonPath, err)
	}
	var m map[string]map[string]string
	err = json.Unmarshal(jsonCnt, &m)
	if err != nil {
		return "", err
	}
	if val, ok := m[lang]; ok {
		docxPath := path.Join(uidPath, val["docx"])
		if _, err := os.Stat(docxPath); err == nil {
			return path.Join(docxPath), nil
		}
	}
	return "", errors.New("SourcesIndex.getDocxPath - Docx not found in index.json.")
}

func (index *SourcesIndex) indexSource(mdbSource *mdbmodels.Source, parents []string, parentIds []int64, authorsByLanguage map[string][]string) *IndexErrors {
	// Create documents in each language with available translation
	i18nMap := make(map[string]Result)
	allLanguages := []string{}
	indexErrors := MakeIndexErrors()
	for _, i18n := range mdbSource.R.SourceI18ns {
		if i18n.Name.Valid && i18n.Name.String != "" {
			indexErrors.ShouldIndex(i18n.Language)
			pathNames := []string{}
			source := Result{
				ResultType:   index.resultType,
				IndexDate:    &utils.Date{Time: time.Now()},
				MDB_UID:      mdbSource.UID,
				FilterValues: KeyValues(consts.ES_UID_TYPE_SOURCE, parents),
				TypedUids:    []string{KeyValue(consts.ES_UID_TYPE_SOURCE, mdbSource.UID)},
			}
			fPath, missingSourceFileErr := index.getDocxPath(mdbSource.UID, i18n.Language)
			// Ignore err here as if missing source for a language in very common and is ok.
			if missingSourceFileErr == nil {
				// Found docx.
				content, parseErr := ParseDocx(fPath)
				indexErrors.DocumentError(i18n.Language, parseErr, fmt.Sprintf("SourcesIndex.indexSource - Error parsing docx for source %s and language %s.  Skipping indexing.", mdbSource.UID, i18n.Language))
				if parseErr == nil {
					source.Content = content
					allLanguages = append(allLanguages, i18n.Language)
				}
			}
			// Find parents...
			findParentsErr := (error)(nil)
			for _, i := range parentIds {
				ni18n, e := mdbmodels.FindSourceI18n(index.db, i, i18n.Language)
				if e != nil {
					if e == sql.ErrNoRows {
						continue
					}
					findParentsErr = e
					break
				}
				if ni18n.Name.Valid && ni18n.Name.String != "" {
					pathNames = append(pathNames, ni18n.Name.String)
				}
			}
			indexErrors.DocumentError(i18n.Language, findParentsErr, "SourcesIndex.indexSource - Error finding parent")
			if findParentsErr != nil {
				indexErrors.DocumentError(i18n.Language, findParentsErr, "SourcesIndex.indexSource - Error finding parent")
				continue
			}
			authors := authorsByLanguage[i18n.Language]
			s := append(authors, pathNames...)
			leaf := s[len(s)-1]
			if i18n.Description.Valid && i18n.Description.String != "" && i18n.Description.String != " " {
				if _, ok := consts.SRC_TYPES_FOR_TITLE_DESCRIPTION_CONCAT[mdbSource.TypeID]; ok {
					// We combine title and description in one field for better support (especialy in intents)
					// of title-subtitle combined queries like:
					// "Part 8 The Eser Sefirot of Olam ha Atzilut"
					// ("Part 8" is the title and "The Eser Sefirot of Olam ha Atzilut" is the description).
					source.Description = fmt.Sprintf("%s %s", leaf, i18n.Description.String)
				} else {
					source.Description = i18n.Description.String
				}
			}
			source.Title = leaf
			suffixes := Suffixes(strings.Join(s, " "))
			if len(s) > 2 {
				suffixes = append(suffixes, ConcateFirstToLast(s))
			}

			if _, ok := consts.ES_SRC_ADD_MAAMAR_TO_SUGGEST[mdbSource.UID]; ok {
				suffixes = append(suffixes, fmt.Sprintf("מאמר %s", leaf))
			}

			//  Add chapter number\letter to Shamati articles
			if mdbSource.ParentID.Valid && mdbSource.Position.Valid && mdbSource.Position.Int > 0 {
				var addPosition bool
				var positionIndexType consts.PositionIndexType
				for _, parent := range parents {
					if val, ok := consts.ES_SRC_PARENTS_FOR_CHAPTER_POSITION_INDEX[parent]; ok {
						addPosition = true
						positionIndexType = val
						break
					}
				}
				if addPosition {
					var position string
					position = strconv.Itoa(mdbSource.Position.Int)
					if i18n.Language == consts.LANG_HEBREW && positionIndexType == consts.LETTER_IF_HEBREW {
						position = utils.NumberInHebrew(mdbSource.Position.Int) //  Convert to Hebrew letter
					} else {
						position = strconv.Itoa(mdbSource.Position.Int)
					}
					// Hebrew example of leaf with position: קלג. אורות דשבת
					// English example of leaf with position: 133. The Lights of Shabbat
					leafWithChapter := fmt.Sprintf("%s. %s", position, leaf)
					s = append(s[:len(s)-1], leafWithChapter)
					suffixesWithChapter := Suffixes(strings.Join(s, " "))
					suffixes = Unique(append(suffixes, suffixesWithChapter...))
				}
			}

			if weight, ok := consts.ES_SUGGEST_SOURCES_WEIGHT[mdbSource.UID]; ok {
				source.TitleSuggest = SuggestField{suffixes, weight}
			} else {
				source.TitleSuggest = SuggestField{suffixes, float64(consts.ES_SOURCES_SUGGEST_DEFAULT_WEIGHT)}
			}

			source.FullTitle = strings.Join(s, " > ")
			i18nMap[i18n.Language] = source
		}
	}

	// Index each document in its language index
	for k, v := range i18nMap {
		v.FilterValues = append(v.FilterValues, KeyValues(consts.FILTER_LANGUAGE, allLanguages)...)
		name := index.IndexName(k)
		log.Debugf("Sources Index - Add source %s to index %s", mdbSource.UID, name)
		resp, err := index.esc.Index().
			Index(name).
			Type("result").
			BodyJson(v).
			Do(context.TODO())
		indexErrors.DocumentError(k, err, fmt.Sprintf("Sources Index - Source %s %s", name, mdbSource.UID))
		if err != nil {
			continue
		}
		errNotCreated := (error)(nil)
		if resp.Result != "created" {
			errNotCreated = errors.New(fmt.Sprintf("Not created: source %s %s", name, mdbSource.UID))
		} else {
			indexErrors.Indexed(k)
		}
		indexErrors.DocumentError(k, errNotCreated, "Sources Index")
	}

	atomic.AddUint64(&index.Progress, 1)
	progress := atomic.LoadUint64(&index.Progress)
	if progress%100 == 0 {
		log.Debugf("Progress sources %d", progress)
	}

	return indexErrors
}
