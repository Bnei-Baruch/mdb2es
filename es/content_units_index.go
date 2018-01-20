package es

import (
	"bytes"
	"context"
    "fmt"
	"database/sql"
	"encoding/json"
    "math"
	"os"
	"os/exec"
	"path"
    "time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/volatiletech/sqlboiler/queries"
	"github.com/volatiletech/sqlboiler/queries/qm"
	"gopkg.in/olivere/elastic.v5"
	log "github.com/Sirupsen/logrus"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/mdb"
	"github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

func MakeContentUnitsIndex(namespace string) *ContentUnitsIndex {
    cui := new(ContentUnitsIndex)
    cui.baseName = consts.ES_UNITS_INDEX
    cui.namespace = namespace
    cui.docFolder = path.Join(viper.GetString("elasticsearch.docx-folder"))
    return cui
}

type ContentUnitsIndex struct {
    BaseIndex
    indexData *IndexData
    docFolder string
}

func (index *ContentUnitsIndex) ReindexAll() error {
    return index.reindex("cu.secure = 0 AND cu.published IS TRUE", false)
}

func (index *ContentUnitsIndex) Reindex(scope Scope) error {
    sqlScope := "cu.secure = 0 AND cu.published IS TRUE"
    if scope.ContentUnitUID != "" {
      sqlScope = fmt.Sprintf("%s AND cu.uid = '%s'", sqlScope, scope.ContentUnitUID)
    }
    return index.reindex(sqlScope, true)
}

func (index *ContentUnitsIndex) reindex(sqlScope string, remove bool) error {
    var units []*mdbmodels.ContentUnit
    err := mdbmodels.NewQuery(mdb.DB,
        qm.From("content_units as cu"),
        qm.Load("ContentUnitI18ns"),
        qm.Load("CollectionsContentUnits"),
        qm.Load("CollectionsContentUnits.Collection"),
        qm.Where(sqlScope)).Bind(&units)
    if err != nil {
        return errors.Wrap(err, "Fetch units from mdb")
    }
    log.Infof("Reindexing %d units (secure and published).", len(units))

    index.indexData = new(IndexData)
    err = index.indexData.Load(sqlScope)
    if err != nil {
        return err
    }

    uids := make([]string, len(units))
    for i, cu := range units {
        uids[i] = cu.UID
    }

    if remove {
        if err := index.RemoveFromIndex(uids); err != nil {
            return err
        }
    }
    for _, unit := range units {
        if err := index.IndexUnit(unit); err != nil {
            return err
        }
    }
    return nil
}

func (index* ContentUnitsIndex) RemoveFromIndex(uids []string) error {
    uidsI := make([]interface{}, len(uids))
    for i, uid := range uids {
        uidsI[i] = uid
    }
	for _, lang := range consts.ALL_KNOWN_LANGS {
		indexName := index.indexName(lang)
		_, err := mdb.ESC.DeleteByQuery(indexName).
            Query(elastic.NewTermsQuery("mdb_uid", uidsI...)).
            Do(context.TODO())
		if err != nil {
            return errors.Wrapf(err, "Remove from index %s %+v\n", indexName, uids)
		}
        // If not exists Deleted will be 0.
		// if resp.Deleted != int64(len(uids)) {
		// 	return errors.Errorf("Not deleted: %s %+v\n", indexName, uids)
		// }
	}

	return nil
}

func (index* ContentUnitsIndex) ParseDocx(uid string) (string, error) {
	docxFilename := fmt.Sprintf("%s.docx", uid)
	docxPath := path.Join(index.docFolder, docxFilename)
	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		return "", nil
	}
	cmd := exec.Command("es/parse_docs.py", docxPath)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		log.Warnf("parse_docs.py %s\nstdout: %s\nstderr: %s", docxPath, stdout.String(), stderr.String())
		return "", errors.Wrapf(err, "cmd.Run %s", uid)
	}
	return stdout.String(), nil
}

func (index* ContentUnitsIndex) collectionsContentTypes(collectionsContentUnits mdbmodels.CollectionsContentUnitSlice) []string {
	ret := make([]string, len(collectionsContentUnits))
	for i, ccu := range collectionsContentUnits {
		ret[i] = mdb.CONTENT_TYPE_REGISTRY.ByID[ccu.R.Collection.TypeID].Name
	}
	return ret
}

func (index* ContentUnitsIndex) IndexUnit(cu *mdbmodels.ContentUnit) error {
    fmt.Printf("IndexUnit: %+v\n", cu)
	// Create documents in each language with available translation
	i18nMap := make(map[string]ContentUnit)
	for i := range cu.R.ContentUnitI18ns {
		i18n := cu.R.ContentUnitI18ns[i]
		if i18n.Name.Valid && i18n.Name.String != "" {
			unit := ContentUnit{
				MDB_UID:                 cu.UID,
				Name:                    i18n.Name.String,
				ContentType:             mdb.CONTENT_TYPE_REGISTRY.ByID[cu.TypeID].Name,
				CollectionsContentTypes: index.collectionsContentTypes(cu.R.CollectionsContentUnits),
			}

			if i18n.Description.Valid && i18n.Description.String != "" {
				unit.Description = i18n.Description.String
			}

			if cu.Properties.Valid {
				var props map[string]interface{}
				err := json.Unmarshal(cu.Properties.JSON, &props)
				if err != nil {
					return errors.Wrapf(err, "json.Unmarshal properties %s", cu.UID)
				}

				if filmDate, ok := props["film_date"]; ok {
					val, err := time.Parse("2006-01-02", filmDate.(string))
					if err != nil {
						return errors.Wrapf(err, "time.Parse film_date %s", cu.UID)
					}
					unit.FilmDate = &utils.Date{Time: val}
				}

				if duration, ok := props["duration"]; ok {
					unit.Duration = uint16(math.Max(0, duration.(float64)))
				}

				if originalLanguage, ok := props["original_language"]; ok {
					unit.OriginalLanguage = originalLanguage.(string)
				}
			}

			if val, ok := index.indexData.Sources[cu.ID]; ok {
				unit.Sources = val
			}
			if val, ok := index.indexData.Tags[cu.ID]; ok {
				unit.Tags = val
			}
			if val, ok := index.indexData.Persons[cu.ID]; ok {
				unit.Persons = val
			}
			if val, ok := index.indexData.Translations[cu.ID]; ok {
				unit.Translations = val
			}
			if byLang, ok := index.indexData.Transcripts[cu.ID]; ok {
				if val, ok := byLang[i18n.Language]; ok {
					var err error
					unit.Transcript, err = index.ParseDocx(val[0])
                    if err != nil {
                        log.Warnf("Error parsing docx: %s", val[0])
                    }
					// if err == nil && unit.Transcript != "" {
					// 	atomic.AddUint64(&withTranscript, 1)
					// }
				}
			}

			i18nMap[i18n.Language] = unit
		}
	}

    fmt.Printf("i18nMap: %+v\n", i18nMap)

	// Index each document in its language index
	for k, v := range i18nMap {
		name := index.indexName(k)
        fmt.Printf("Indexing to %s: %+v\n", name, v)
		resp, err := mdb.ESC.Index().
			Index(name).
			Type("content_units").
			BodyJson(v).
			Do(context.TODO())
		if err != nil {
			return errors.Wrapf(err, "Index unit %s %s", name, cu.UID)
		}
		if !resp.Created {
			return errors.Errorf("Not created: unit %s %s", name, cu.UID)
		}
	}

	return nil
}

type IndexData struct {
	Sources      map[int64][]string
	Tags         map[int64][]string
	Persons      map[int64][]string
	Translations map[int64][]string
	Transcripts  map[int64]map[string][]string
}

func (cm *IndexData) Load(sqlScope string) error {
	var err error

	cm.Sources, err = cm.loadSources(sqlScope)
	if err != nil {
		return err
	}

	cm.Tags, err = cm.loadTags(sqlScope)
	if err != nil {
		return err
	}

	cm.Persons, err = cm.loadPersons(sqlScope)
	if err != nil {
		return err
	}

	cm.Translations, err = cm.loadTranslations(sqlScope)
	if err != nil {
		return err
	}

	cm.Transcripts, err = cm.loadTranscripts(sqlScope)
	if err != nil {
		return err
	}

	return nil
}

func (cm *IndexData) loadSources(sqlScope string) (map[int64][]string, error) {
	rows, err := queries.Raw(mdb.DB, fmt.Sprintf(`
WITH RECURSIVE rec_sources AS (
  SELECT
    s.id,
    s.uid,
    s.position,
    ARRAY [a.code, s.uid] "path"
  FROM sources s INNER JOIN authors_sources aas ON s.id = aas.source_id
    INNER JOIN authors a ON a.id = aas.author_id
  UNION
  SELECT
    s.id,
    s.uid,
    s.position,
    rs.path || s.uid
  FROM sources s INNER JOIN rec_sources rs ON s.parent_id = rs.id
)
SELECT
  cus.content_unit_id,
  array_agg(DISTINCT item)
FROM content_units_sources cus
    INNER JOIN rec_sources AS rs ON cus.source_id = rs.id
    INNER JOIN content_units AS cu ON cus.content_unit_id = cu.id
    , unnest(rs.path) item
WHERE %s
GROUP BY cus.content_unit_id;`, sqlScope)).Query()

	if err != nil {
		return nil, errors.Wrap(err, "Load sources")
	}
	defer rows.Close()

	return cm.loadMap(rows)
}

func (cm *IndexData) loadTags(sqlScope string) (map[int64][]string, error) {
	rows, err := queries.Raw(mdb.DB, fmt.Sprintf(`
WITH RECURSIVE rec_tags AS (
  SELECT
    t.id,
    t.uid,
    ARRAY [t.uid] :: CHAR(8) [] "path"
  FROM tags t
  WHERE parent_id IS NULL
  UNION
  SELECT
    t.id,
    t.uid,
    (rt.path || t.uid) :: CHAR(8) []
  FROM tags t INNER JOIN rec_tags rt ON t.parent_id = rt.id
)
SELECT
  cut.content_unit_id,
  array_agg(DISTINCT item)
FROM content_units_tags cut
    INNER JOIN rec_tags AS rt ON cut.tag_id = rt.id
    INNER JOIN content_units AS cu ON cut.content_unit_id = cu.id
    , unnest(rt.path) item
WHERE %s
GROUP BY cut.content_unit_id;`, sqlScope)).Query()

	if err != nil {
		return nil, errors.Wrap(err, "Load tags")
	}
	defer rows.Close()

	return cm.loadMap(rows)
}

func (cm *IndexData) loadPersons(sqlScope string) (map[int64][]string, error) {
	rows, err := queries.Raw(mdb.DB, fmt.Sprintf(`
SELECT
  cup.content_unit_id,
  array_agg(p.uid)
FROM content_units_persons cup
    INNER JOIN persons p ON cup.person_id = p.id
    INNER JOIN content_units AS cu ON cup.content_unit_id = cu.id
WHERE %s
GROUP BY cup.content_unit_id;`, sqlScope)).Query()

	if err != nil {
		return nil, errors.Wrap(err, "Load persons")
	}
	defer rows.Close()

	return cm.loadMap(rows)
}

func (cm *IndexData) loadTranslations(sqlScope string) (map[int64][]string, error) {
	rows, err := queries.Raw(mdb.DB, fmt.Sprintf(`
SELECT
  content_unit_id,
  array_agg(DISTINCT language)
FROM files
    INNER JOIN content_units AS cu ON files.content_unit_id = cu.id
WHERE language NOT IN ('zz', 'xx') AND content_unit_id IS NOT NULL AND %s
GROUP BY content_unit_id;`, sqlScope)).Query()

	if err != nil {
		return nil, errors.Wrap(err, "Load translations")
	}
	defer rows.Close()

	return cm.loadMap(rows)
}

func (cm *IndexData) loadMap(rows *sql.Rows) (map[int64][]string, error) {
	m := make(map[int64][]string)

	for rows.Next() {
		var cuid int64
		var sources pq.StringArray
		err := rows.Scan(&cuid, &sources)
		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}
		m[cuid] = sources
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows.Err()")
	}

	return m, nil
}

func (cm *IndexData) loadTranscripts(sqlScope string) (map[int64]map[string][]string, error) {
	rows, err := queries.Raw(mdb.DB, fmt.Sprintf(`
SELECT
    f.uid,
    f.name,
    f.language,
    cu.id
FROM files AS f
    INNER JOIN content_units AS cu ON f.content_unit_id = cu.id
WHERE name ~ '.docx?' AND
    f.language NOT IN ('zz', 'xx') AND
    f.content_unit_id IS NOT NULL AND
    cu.type_id != 31 AND
    %s;`, sqlScope)).Query()

	if err != nil {
		return nil, errors.Wrap(err, "Load transcripts")
	}
	defer rows.Close()

	return loadTranscriptsMap(rows)
}

func loadTranscriptsMap(rows *sql.Rows) (map[int64]map[string][]string, error) {
	m := make(map[int64]map[string][]string)

	for rows.Next() {
		var uid string
		var name string
		var language string
		var cuID int64
		err := rows.Scan(&uid, &name, &language, &cuID)
		if err != nil {
			return nil, errors.Wrap(err, "rows.Scan")
		}
		if _, ok := m[cuID]; !ok {
			m[cuID] = make(map[string][]string)
		}
		m[cuID][language] = []string{uid, name}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "rows.Err()")
	}

	return m, nil
}
