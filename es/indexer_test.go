package es_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Bnei-Baruch/sqlboiler/boil"
	"github.com/Bnei-Baruch/sqlboiler/queries/qm"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/olivere/elastic.v6"
	"gopkg.in/volatiletech/null.v6"

	"github.com/Bnei-Baruch/archive-backend/common"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/Bnei-Baruch/archive-backend/mdb"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

type IndexerSuite struct {
	suite.Suite
	utils.TestDBManager
	esc             *elastic.Client
	ctx             context.Context
	server          *httptest.Server
	serverResponses map[string]string
}

type Collection struct {
	MDB_UID                  string      `json:"mdb_uid"`
	TypedUIDs                []string    `json:"typed_uids"`
	Name                     string      `json:"name"`
	Description              string      `json:"description"`
	ContentType              string      `json:"content_type"`
	ContentUnitsContentTypes []string    `json:"content_units_content_types,omitempty"`
	EffectiveDate            *utils.Date `json:"effective_date"`
	OriginalLanguage         string      `json:"original_language,omitempty"`
}

type ContentUnit struct {
	MDB_UID                 string      `json:"mdb_uid"`
	TypedUIDs               []string    `json:"typed_uids"`
	Name                    string      `json:"name,omitempty"`
	Description             string      `json:"description,omitempty"`
	ContentType             string      `json:"content_type"`
	CollectionsContentTypes []string    `json:"collections_content_types,omitempty"`
	EffectiveDate           *utils.Date `json:"effective_date,omitempty"`
	Duration                uint64      `json:"duration,omitempty"`
	OriginalLanguage        string      `json:"original_language,omitempty"`
	Translations            []string    `json:"translations,omitempty"`
	Tags                    []string    `json:"tags,omitempty"`
	Sources                 []string    `json:"sources,omitempty"`
	Authors                 []string    `json:"authors,omitempty"`
	Persons                 []string    `json:"persons,omitempty"`
	Transcript              string      `json:"transcript,omitempty"`
}

type File struct {
	MDB_UID  string      `json:"mdb_uid"`
	Name     string      `json:"name"`
	Size     uint64      `json:"size"`
	URL      string      `json:"url"`
	Secure   int         `json:"secure"`
	FilmDate *utils.Date `json:"film_date"`
	Duration float64     `json:"duration,omitempty"`
	Language string      `json:"language,omitempty"`
	MimeType string      `json:"mimetype,omitempty"`
	Type     string      `json:"type,omitempty"`
	SubType  string      `json:"subtype,omitempty"`
}

type Classification struct {
	MDB_UID            string `json:"mdb_uid"`
	Name               string `json:"name,omitempty"`
	NameSuggest        string `json:"name_suggest,omitempty"`
	Description        string `json:"description,omitempty"`
	DescriptionSuggest string `json:"description_suggest,omitempty"`
	Type               string `json:"classification_type"`
}

type Source struct {
	MDB_UID string `json:"mdb_uid"`
	Name    string `json:"name"`

	// Deprecated fields (since we use 'Result Template' in order to index the sources):
	Description string     `json:"description"`
	Content     string     `json:"content"`
	Sources     []string   `json:"sources"`
	Authors     []string   `json:"authors"`
	PathNames   []string   `json:"path_names"`
	FullName    []string   `json:"full_name"`
	ParentID    null.Int64 `json:"parent_id"`
	Position    null.Int   `json:"position"`
}

func (suite *IndexerSuite) SetupSuite() {
	utils.InitConfig("", "../")
	err := suite.InitTestDB()
	if err != nil {
		panic(err)
	}
	suite.ctx = context.Background()

	fmt.Println("Replace sources folder with temp. path.")
	testingsSourcesFolder := viper.GetString("test.test-sources-folder")
	viper.Set("elasticsearch.sources-folder", testingsSourcesFolder)

	fmt.Println("Replace unzip-url with test.")
	suite.serverResponses = make(map[string]string)
	handler := func(w http.ResponseWriter, r *http.Request) {
		key := ""
		if r.URL.Path != "" {
			key += fmt.Sprintf("%s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			key += fmt.Sprintf("?%s", r.URL.RawQuery)
		}
		if r.URL.Fragment != "" {
			key += fmt.Sprintf("#%s", r.URL.Fragment)
		}
		w.Header().Set("Content-Type", "plain/text")
		if val, ok := suite.serverResponses[key]; ok {
			fmt.Printf("LOOKUP KEY [%s]\tRESPONSE [%s]\n", key, val)
			io.WriteString(w, val)
		} else {
			fmt.Printf("LOOKUP KEY [%s]\tNot found!\n", key)
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "Lookup not found.")
		}
	}
	suite.server = httptest.NewServer(http.HandlerFunc(handler))
	viper.Set("elasticsearch.unzip-url", suite.server.URL)

	// Set package db and esc variables.
	common.InitWithDefault(suite.DB)
	boil.DebugMode = viper.GetString("boiler-mode") == "debug"
	esc, err := common.ESC.GetClient()
	if err != nil {
		panic(err)
	}
	suite.esc = esc
}

func (suite *IndexerSuite) TearDownSuite() {
	// Close mock server.
	suite.server.Close()
	// Close connections.
	common.Shutdown()
	// Drop test database.
	suite.Require().Nil(suite.DestroyTestDB())
}

type ESLogAdapter struct{ *testing.T }

func (s ESLogAdapter) Printf(format string, v ...interface{}) { s.Logf(format, v...) }

// MOVE THIS TO EACH RESULT TYPE TEST!
// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
// func TestIndexer(t *testing.T) {
// 	suite.Run(t, new(IndexerSuite))
// }

func (suite *IndexerSuite) SetupTest() {
	r := require.New(suite.T())
	units, err := mdbmodels.ContentUnits(common.DB).All()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var uids []string
	for _, unit := range units {
		uids = append(uids, unit.UID)
	}
	r.Nil(deleteContentUnits(uids))
	// Remove test indexes.
	indexer, err := es.MakeIndexer("test", "test-date", []string{consts.ES_RESULT_TYPE_UNITS}, common.DB, esc)
	r.Nil(err)
	r.Nil(indexer.DeleteIndexes())
	// Delete test directory
	utils.Must(os.MkdirAll(viper.GetString("test.test-sources-folder"), 0777))
}

func updateCollection(c Collection, cuUID string, removeContentUnitUID string) (string, error) {
	var mdbCollection mdbmodels.Collection
	if c.MDB_UID != "" {
		cp, err := mdbmodels.Collections(common.DB, qm.Where("uid = ?", c.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbCollection = *cp
	} else {
		mdbCollection = mdbmodels.Collection{
			UID:    utils.GenerateUID(8),
			TypeID: mdb.CONTENT_TYPE_REGISTRY.ByName[c.ContentType].ID,
		}
		if err := mdbCollection.Insert(common.DB); err != nil {
			return "", err
		}
	}
	if c.ContentType != "" {
		mdbCollection.TypeID = mdb.CONTENT_TYPE_REGISTRY.ByName[c.ContentType].ID
	}
	mdbCollection.Secure = int16(0)
	mdbCollection.Published = true
	if err := mdbCollection.Update(common.DB); err != nil {
		return "", err
	}
	// I18N
	var mdbCollectionI18n mdbmodels.CollectionI18n
	lang := consts.LANG_ENGLISH
	ci18np, err := mdbmodels.FindCollectionI18n(common.DB, mdbCollection.ID, lang)
	if err == sql.ErrNoRows {
		mdbCollectionI18n = mdbmodels.CollectionI18n{
			CollectionID: mdbCollection.ID,
			Language:     lang,
		}
		if err := mdbCollectionI18n.Insert(common.DB); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		mdbCollectionI18n = *ci18np
	}
	if c.Name != "" {
		mdbCollectionI18n.Name = null.NewString(c.Name, c.Name != "")
	}
	if c.Description != "" {
		mdbCollectionI18n.Description = null.NewString(c.Description, c.Description != "")
	}
	if err := mdbCollectionI18n.Update(common.DB); err != nil {
		return "", err
	}

	cu, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cuUID)).One()
	if err != nil {
		return "", err
	}
	if _, err := mdbmodels.FindCollectionsContentUnit(common.DB, mdbCollection.ID, cu.ID); err == sql.ErrNoRows {
		var mdbCollectionsContentUnit mdbmodels.CollectionsContentUnit
		mdbCollectionsContentUnit.CollectionID = mdbCollection.ID
		mdbCollectionsContentUnit.ContentUnitID = cu.ID
		if err := mdbCollectionsContentUnit.Insert(common.DB); err != nil {
			return "", err
		}
	}
	// Remomove only the connection between the collection and this content unit.
	if removeContentUnitUID != "" {
		ccus, err := mdbmodels.CollectionsContentUnits(common.DB,
			qm.InnerJoin("content_units on content_units.id = collections_content_units.content_unit_id"),
			qm.Where("content_units.uid = ?", removeContentUnitUID),
			qm.And("collection_id = ?", mdbCollection.ID)).All()
		if err != nil {
			return "", errors.Wrap(err, "updateCollection select ccu")
		}
		for _, ccu := range ccus {
			if err := mdbmodels.CollectionsContentUnits(common.DB,
				qm.Where("collection_id = ?", ccu.CollectionID),
				qm.And("content_unit_id = ?", ccu.ContentUnitID)).DeleteAll(); err != nil {
				return "", errors.Wrap(err, "updateCollection delete ccu")
			}
		}
	}
	return mdbCollection.UID, nil
}

func (suite *IndexerSuite) uc(c Collection, cuUID string, removeContentUnitUID string) string {
	r := require.New(suite.T())
	uid, err := updateCollection(c, cuUID, removeContentUnitUID)
	r.Nil(err)
	return uid
}

func removeContentUnitTag(cu ContentUnit, lang string, tag mdbmodels.Tag) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		return "", errors.New("cu.MDB_UID is empty")
	}

	_, err := mdbmodels.FindTag(common.DB, tag.ID)
	if err != nil {
		return "", err
	}

	err = mdbContentUnit.RemoveTags(common.DB, &tag)
	if err != nil {
		return "", err
	}

	return mdbContentUnit.UID, nil
}

func addContentUnitTag(cu ContentUnit, lang string, tag mdbmodels.Tag) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		mdbContentUnit = mdbmodels.ContentUnit{
			UID:    utils.GenerateUID(8),
			TypeID: mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LESSON_PART].ID,
		}
		if err := mdbContentUnit.Insert(common.DB); err != nil {
			return "", err
		}
	}

	_, err := mdbmodels.FindTag(common.DB, tag.ID)
	if err != nil {
		if err == sql.ErrNoRows {

			// save tag to DB:

			/*//generate uid
			b := make([]byte, 8)
			for i := range b {
				b[i] = uidBytes[rand.Intn(len(uidBytes))]
			}
			tag.UID = string(b)*/

			err = tag.Insert(common.DB)
			if err != nil {
				return "", err
			}

			// save i18n
			/*for _, v := range tag.I18n {
				err := t.AddTagI18ns(exec, true, v)
				if err != nil {
					return "", err
				}
			}*/

		} else {
			return "", err
		}
	}

	err = mdbContentUnit.AddTags(common.DB, false, &tag)
	if err != nil {
		return "", err
	}

	return mdbContentUnit.UID, nil
}

func addContentUnitSource(cu ContentUnit, lang string, src mdbmodels.Source, author mdbmodels.Author, insertAuthor bool) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		mdbContentUnit = mdbmodels.ContentUnit{
			UID:    utils.GenerateUID(8),
			TypeID: mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LESSON_PART].ID,
		}
		if err := mdbContentUnit.Insert(common.DB); err != nil {
			return "", err
		}
	}

	_, err := mdbmodels.FindSource(common.DB, src.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = src.Insert(common.DB)
			if err != nil {
				return "", err
			}
			err = src.AddAuthors(common.DB, insertAuthor, &author)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	err = mdbContentUnit.AddSources(common.DB, false, &src)
	if err != nil {
		return "", err
	}

	return mdbContentUnit.UID, nil
}

func removeContentUnitSource(cu ContentUnit, lang string, src mdbmodels.Source) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		return "", errors.New("cu.MDB_UID is empty")
	}

	_, err := mdbmodels.FindSource(common.DB, src.ID)
	if err != nil {
		return "", err
	}

	err = mdbContentUnit.RemoveSources(common.DB, &src)
	if err != nil {
		return "", err
	}

	return mdbContentUnit.UID, nil
}

func addContentUnitFile(cu ContentUnit, lang string, file mdbmodels.File) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		mdbContentUnit = mdbmodels.ContentUnit{
			UID:    utils.GenerateUID(8),
			TypeID: mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LESSON_PART].ID,
		}
		if err := mdbContentUnit.Insert(common.DB); err != nil {
			return "", err
		}
	}

	_, err := mdbmodels.FindFile(common.DB, file.ID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = file.Insert(common.DB)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}

	err = mdbContentUnit.AddFiles(common.DB, false, &file)
	if err != nil {
		return "", err
	}

	return file.UID, nil
}

func removeContentUnitFile(cu ContentUnit, lang string, file mdbmodels.File) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		return "", errors.New("cu.MDB_UID is empty")
	}

	_, err := mdbmodels.FindFile(common.DB, file.ID)
	if err != nil {
		return "", err
	}

	err = mdbContentUnit.RemoveFiles(common.DB, &file)
	if err != nil {
		return "", err
	}

	return file.UID, nil
}

func setContentUnitType(cu ContentUnit, typeID int64) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		return "", errors.New("cu.MDB_UID is empty")
	}
	mdbContentUnit.TypeID = typeID

	if err := mdbContentUnit.Update(common.DB); err != nil {
		return "", err
	}
	return mdbContentUnit.UID, nil
}

func updateContentUnit(cu ContentUnit, lang string, published bool, secure bool) (string, error) {
	var mdbContentUnit mdbmodels.ContentUnit
	if cu.MDB_UID != "" {
		cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cu.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbContentUnit = *cup
	} else {
		mdbContentUnit = mdbmodels.ContentUnit{
			UID:    utils.GenerateUID(8),
			TypeID: mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LESSON_PART].ID,
		}
		if err := mdbContentUnit.Insert(common.DB); err != nil {
			return "", err
		}
	}
	s := int16(0)
	if !secure {
		s = int16(1)
	}
	p := true
	if !published {
		p = false
	}

	mdbContentUnit.Secure = s
	mdbContentUnit.Published = p
	if err := mdbContentUnit.Update(common.DB); err != nil {
		return "", err
	}
	var mdbContentUnitI18n mdbmodels.ContentUnitI18n
	cui18np, err := mdbmodels.FindContentUnitI18n(common.DB, mdbContentUnit.ID, lang)
	if err == sql.ErrNoRows {
		mdbContentUnitI18n = mdbmodels.ContentUnitI18n{
			ContentUnitID: mdbContentUnit.ID,
			Language:      lang,
		}
		if err := mdbContentUnitI18n.Insert(common.DB); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		mdbContentUnitI18n = *cui18np
	}
	if cu.Name != "" {
		mdbContentUnitI18n.Name = null.NewString(cu.Name, cu.Name != "")
	}
	if cu.Description != "" {
		mdbContentUnitI18n.Description = null.NewString(cu.Description, cu.Description != "")
	}
	if err := mdbContentUnitI18n.Update(common.DB); err != nil {
		return "", err
	}
	return mdbContentUnit.UID, nil
}

func updateFile(f File, cuUID string) (string, error) {
	cup, err := mdbmodels.ContentUnits(common.DB, qm.Where("uid = ?", cuUID)).One()
	if err != nil {
		return "", err
	}
	var mdbFile mdbmodels.File
	if f.MDB_UID != "" {
		fp, err := mdbmodels.Files(common.DB, qm.Where("uid = ?", f.MDB_UID)).One()
		if err != nil {
			return "", err
		}
		mdbFile = *fp
	} else {
		mdbFile = mdbmodels.File{
			UID: utils.GenerateUID(8),
		}
		if err := mdbFile.Insert(common.DB); err != nil {
			return "", err
		}
	}
	mdbFile.Name = f.Name
	mdbFile.ContentUnitID = null.Int64{cup.ID, true}
	if err := mdbFile.Update(common.DB); err != nil {
		return "", err
	}
	return mdbFile.UID, nil
}

func deleteCollection(UID string) error {
	collectionsI18ns, err := mdbmodels.CollectionI18ns(common.DB,
		qm.InnerJoin("collections on collections.id = collection_i18n.collection_id"),
		qm.WhereIn("collections.uid = ?", UID)).All()
	if err != nil {
		return errors.Wrap(err, "deleteCollections, select cu i18n.")
	}
	idsI := make([]interface{}, len(collectionsI18ns))
	for i, v := range collectionsI18ns {
		idsI[i] = v.CollectionID
	}
	if len(collectionsI18ns) > 0 {
		if err := mdbmodels.CollectionI18ns(common.DB, qm.WhereIn("collection_id in ?", idsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteCollections, delete cu i18n.")
		}
	}
	ccu, err := mdbmodels.CollectionsContentUnits(common.DB,
		qm.InnerJoin("collections on collections.id = collections_content_units.collection_id"),
		qm.Where("collections.uid = ?", UID)).All()
	if err != nil {
		return err
	}
	ccu.DeleteAll(common.DB)
	return mdbmodels.Collections(common.DB, qm.Where("uid = ?", UID)).DeleteAll()
}

func deleteCollections(UIDs []string) error {
	for _, uid := range UIDs {
		err := deleteCollection(uid)
		if err != nil {
			return err
		}
	}
	return nil
}

func deletePosts(IDs []string) error {

	quoted := make([]string, len(IDs))
	for i, id := range IDs {
		s := strings.Split(id, "-")
		blogId := s[0]
		wpId := s[1]
		quoted[i] = fmt.Sprintf("(blog_id = %s and wp_id = %s)", blogId, wpId)
	}
	scope := strings.Join(quoted, " or ")

	return mdbmodels.BlogPosts(common.DB, qm.WhereIn(scope)).DeleteAll()
}

func deleteTweets(TIDs []string) error {
	TIDsI := make([]interface{}, len(TIDs))
	for i, v := range TIDs {
		TIDsI[i] = v
	}
	return mdbmodels.TwitterTweets(common.DB, qm.WhereIn("twitter_id in ?", TIDsI...)).DeleteAll()
}

func deleteContentUnits(UIDs []string) error {
	if len(UIDs) == 0 {
		return nil
	}
	UIDsI := make([]interface{}, len(UIDs))
	for i, v := range UIDs {
		UIDsI[i] = v
	}
	files, err := mdbmodels.Files(common.DB,
		qm.InnerJoin("content_units on content_units.id = files.content_unit_id"),
		qm.WhereIn("content_units.uid in ?", UIDsI...)).All()
	if err != nil {
		return errors.Wrap(err, "deleteContentUnits, Select files.")
	}
	fileIdsI := make([]interface{}, len(files))
	for i, v := range files {
		fileIdsI[i] = v.ContentUnitID
	}
	if len(files) > 0 {
		if err := mdbmodels.Files(common.DB, qm.WhereIn("content_unit_id in ?", fileIdsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteContentUnits, delete files.")
		}
	}
	contentUnitsI18ns, err := mdbmodels.ContentUnitI18ns(common.DB,
		qm.InnerJoin("content_units on content_units.id = content_unit_i18n.content_unit_id"),
		qm.WhereIn("content_units.uid in ?", UIDsI...)).All()
	if err != nil {
		return errors.Wrap(err, "deleteContentUnits, select cu i18n.")
	}
	idsI := make([]interface{}, len(contentUnitsI18ns))
	for i, v := range contentUnitsI18ns {
		idsI[i] = v.ContentUnitID
	}
	if len(contentUnitsI18ns) > 0 {
		if err := mdbmodels.ContentUnitI18ns(common.DB, qm.WhereIn("content_unit_id in ?", idsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteContentUnits, delete cu i18n.")
		}
	}
	collectionIds, err := mdbmodels.CollectionsContentUnits(common.DB,
		qm.InnerJoin("content_units on content_units.id = collections_content_units.content_unit_id"),
		qm.WhereIn("content_units.uid IN ?", UIDsI...)).All()
	if err != nil {
		return errors.Wrap(err, "deleteContentUnits, select ccu.")
	}
	if len(collectionIds) > 0 {
		collectionIdsI := make([]interface{}, len(collectionIds))
		for i, v := range collectionIds {
			collectionIdsI[i] = v.CollectionID
		}
		if err := mdbmodels.CollectionsContentUnits(common.DB,
			qm.WhereIn("collection_id IN ?", collectionIdsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteContentUnits, delete ccu.")
		}
		if err := mdbmodels.CollectionI18ns(common.DB,
			qm.WhereIn("collection_id IN ?", collectionIdsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteContentUnitw, delete c i18n.")
		}
		if err := mdbmodels.Collections(common.DB,
			qm.WhereIn("id IN ?", collectionIdsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteContentUnits, delete collections.")
		}
	}
	return mdbmodels.ContentUnits(common.DB, qm.WhereIn("uid in ?", UIDsI...)).DeleteAll()
}

func deleteSources(UIDs []string) error {
	if len(UIDs) == 0 {
		return nil
	}
	UIDsI := make([]interface{}, len(UIDs))
	for i, v := range UIDs {
		UIDsI[i] = v
	}
	sourcesI18ns, err := mdbmodels.SourceI18ns(common.DB,
		qm.InnerJoin("sources on sources.id = source_i18n.source_id"),
		qm.WhereIn("sources.uid in ?", UIDsI...)).All()
	if err != nil {
		return errors.Wrap(err, "deleteSources, select source i18n.")
	}
	idsI := make([]interface{}, len(sourcesI18ns))
	for i, v := range sourcesI18ns {
		idsI[i] = v.SourceID
	}
	if len(sourcesI18ns) > 0 {
		if err := mdbmodels.SourceI18ns(common.DB, qm.WhereIn("source_id in ?", idsI...)).DeleteAll(); err != nil {
			return errors.Wrap(err, "deleteSources, delete source i18n.")
		}
	}
	// Delete authors_sources and authors
	sources, err := mdbmodels.Sources(common.DB, qm.WhereIn("sources.uid in ?", UIDsI...)).All()
	if err != nil {
		return err
	}
	sourcesIdsI := make([]interface{}, 0)
	for _, s := range sources {
		sourcesIdsI = append(sourcesIdsI, s.ID)
	}
	authors, err := mdbmodels.Authors(common.DB,
		qm.InnerJoin("authors_sources on authors_sources.author_id = authors.id"),
		qm.WhereIn("authors_sources.source_id in ?", sourcesIdsI...)).All()
	if err != nil {
		return errors.Wrap(err, "Select authors")
	}
	if len(authors) > 0 {
		for _, s := range sources {
			err = s.RemoveAuthors(common.DB, authors...)
			if err != nil {
				return errors.Wrap(err, "RemoveAuthors")
			}
		}
		err = authors.DeleteAll(common.DB)
		if err != nil {
			return errors.Wrap(err, "deleteSources, delete authors.")
		}
	}
	return mdbmodels.Sources(common.DB, qm.WhereIn("uid in ?", UIDsI...)).DeleteAll()
}

func updateTag(id int64, parentId null.Int64, name string, language string) (string, error) {
	tag, err := mdbmodels.FindTag(common.DB, id)
	if err != nil {
		if err == sql.ErrNoRows {
			tag = &mdbmodels.Tag{
				ID:       id,
				UID:      utils.GenerateUID(8),
				ParentID: parentId,
			}
			err = tag.Insert(common.DB)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	tagI18n, err := mdbmodels.FindTagI18n(common.DB, tag.ID, language)
	if err != nil {
		if err == sql.ErrNoRows {
			tagI18n := mdbmodels.TagI18n{
				TagID:    id,
				Language: language,
				Label:    null.String{Valid: true, String: name},
			}
			err := tag.AddTagI18ns(common.DB, true, &tagI18n)
			if err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	} else {
		tagI18n.Label = null.String{Valid: true, String: name}
		tagI18n.Update(common.DB)
	}
	return tag.UID, nil
}

func removeTag(id int64) error {
	if err := mdbmodels.TagI18ns(common.DB, qm.WhereIn("tag_id = ?", id)).DeleteAll(); err != nil {
		return err
	}
	return mdbmodels.Tags(common.DB, qm.WhereIn("id = ?", id)).DeleteAll()
}

//Return values: MDB_UID - key of the source in string interpretation
//				 MDB_ID - key of the source in int64 interpretation
func updateSource(source Source, lang string) (string, int64, error) {
	var mdbSource mdbmodels.Source
	if source.MDB_UID != "" {
		s, err := mdbmodels.Sources(common.DB, qm.Where("uid = ?", source.MDB_UID)).One()
		if err != nil {
			return "", 0, err
		}
		mdbSource = *s
		if source.Position.Valid {
			mdbSource.Position = source.Position
		}

		if source.ParentID.Valid {
			mdbSource.ParentID = source.ParentID
		}

		if err := mdbSource.Update(common.DB); err != nil {
			return "", 0, err
		}
	} else {
		mdbSource = mdbmodels.Source{
			UID:    utils.GenerateUID(8),
			TypeID: 2,
			Name:   source.Name,
		}
		if source.Description != "" {
			mdbSource.Description = null.NewString(source.Description, source.Description != "")
		}
		if source.ParentID.Valid {
			mdbSource.ParentID = source.ParentID
		}
		if source.Position.Valid {
			mdbSource.Position = source.Position
		}

		if err := mdbSource.Insert(common.DB); err != nil {
			return "", 0, err
		}
	}
	var mdbSourceI18n mdbmodels.SourceI18n
	source18np, err := mdbmodels.FindSourceI18n(common.DB, mdbSource.ID, lang)
	if err == sql.ErrNoRows {
		mdbSourceI18n = mdbmodels.SourceI18n{
			SourceID: mdbSource.ID,
			Language: lang,
		}
		if err := mdbSourceI18n.Insert(common.DB); err != nil {
			return "", 0, err
		}
	} else if err != nil {
		return "", 0, err
	} else {
		mdbSourceI18n = *source18np
	}
	if source.Name != "" {
		mdbSourceI18n.Name = null.NewString(source.Name, source.Name != "")
	}
	if source.Description != "" {
		mdbSourceI18n.Description = null.NewString(source.Description, source.Description != "")
	}
	if err := mdbSourceI18n.Update(common.DB); err != nil {
		return "", 0, err
	}

	// Add folder for source files
	uidPath := path.Join(es.SourcesFolder(), mdbSource.UID)
	if _, err := os.Stat(uidPath); os.IsNotExist(err) {
		err = os.MkdirAll(uidPath, os.FileMode(0775))
		if err != nil {
			return "", 0, err
		}
	}

	return mdbSource.UID, mdbSource.ID, nil
}

func updateSourceFileContent(uid string, lang string) error {
	uidPath := path.Join(es.SourcesFolder(), uid)
	jsonPath := path.Join(uidPath, "index.json")
	contentFileName := fmt.Sprintf("sample-content-%s.docx", lang)
	contentPath := path.Join(uidPath, contentFileName)
	m := make(map[string]map[string]string)

	if _, err := os.Stat(jsonPath); err == nil {
		jsonCnt, err := ioutil.ReadFile(jsonPath)
		if err != nil {
			return fmt.Errorf("Unable to read from file %s. Error: %+v", jsonPath, err)
		}
		err = json.Unmarshal(jsonCnt, &m)
		if err != nil {
			return err
		}
	}

	m[lang] = make(map[string]string)
	m[lang]["docx"] = contentFileName

	newJsonCnt, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("Cannot serialize to Json. Error: %+v", err)
	}

	err = ioutil.WriteFile(jsonPath, newJsonCnt, 0666)
	if err != nil {
		return fmt.Errorf("Unable to write into file %s. Error: %+v", jsonPath, err)
	}

	fileToCopy := viper.GetString("test.test-source-content-docx")
	data, err := ioutil.ReadFile(fileToCopy)
	if err != nil {
		return fmt.Errorf("Unable to read file %s. Error: %+v", fileToCopy, err)
	}
	err = ioutil.WriteFile(contentPath, data, 0644)
	if err != nil {
		return fmt.Errorf("Unable to write into file %s. Error: %+v", contentPath, err)
	}

	return nil
}

func addAuthorToSource(source Source, lang string, mdbAuthor mdbmodels.Author, insertAuthor bool, insertI18n bool) error {
	var mdbSource mdbmodels.Source
	if source.MDB_UID != "" {
		src, err := mdbmodels.Sources(common.DB, qm.Where("uid = ?", source.MDB_UID)).One()
		if err != nil {
			return err
		}
		mdbSource = *src
	} else {
		mdbSource = mdbmodels.Source{
			UID:    utils.GenerateUID(8),
			TypeID: 2,
		}
		if err := mdbSource.Insert(common.DB); err != nil {
			return err
		}
	}

	err := mdbSource.AddAuthors(common.DB, insertAuthor, &mdbAuthor)
	if err != nil {
		return err
	}

	if insertI18n {
		var mdbAuthorI18n mdbmodels.AuthorI18n
		author18n, err := mdbmodels.FindAuthorI18n(common.DB, mdbAuthor.ID, lang)
		if err == sql.ErrNoRows {
			mdbAuthorI18n = mdbmodels.AuthorI18n{
				AuthorID: mdbAuthor.ID,
				Language: lang,
			}
			if err := mdbAuthorI18n.Insert(common.DB); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			mdbAuthorI18n = *author18n
		}
		if mdbAuthor.Name != "" {
			mdbAuthorI18n.Name = null.NewString(mdbAuthor.Name, mdbAuthor.Name != "")
		}
		if mdbAuthor.FullName.Valid && mdbAuthor.FullName.String != "" {
			mdbAuthorI18n.FullName = null.NewString(mdbAuthor.FullName.String, true)
		}
		if err := mdbAuthorI18n.Update(common.DB); err != nil {
			return err
		}
	}

	return nil
}

func removeAuthorFromSource(source Source, mdbAuthor mdbmodels.Author) error {
	var mdbSource mdbmodels.Source
	if source.MDB_UID != "" {
		src, err := mdbmodels.Sources(common.DB, qm.Where("uid = ?", source.MDB_UID)).One()
		if err != nil {
			return err
		}
		mdbSource = *src
	} else {
		return errors.New("source.MDB_UID is empty")
	}

	_, err := mdbmodels.FindAuthor(common.DB, mdbAuthor.ID)
	if err != nil {
		return err
	}

	err = mdbSource.RemoveAuthors(common.DB, &mdbAuthor)
	if err != nil {
		return err
	}

	return nil
}

func insertPost(wp_id int64, blogId int64, title string, filtered bool) error {

	mdbPost := mdbmodels.BlogPost{
		ID:        wp_id,
		BlogID:    blogId,
		WPID:      wp_id,
		Title:     title,
		Content:   "",
		PostedAt:  time.Now(),
		CreatedAt: time.Now(),
		Link:      "",
		Filtered:  filtered,
	}
	if err := mdbPost.Insert(common.DB); err != nil {
		return err
	}
	return nil
}

func insertTweet(id int64, tid string, userId int64, title string) error {

	_, err := mdbmodels.FindTwitterUser(common.DB, userId)
	if err != nil {
		if err == sql.ErrNoRows {

			// save twitter user to DB:
			usr := mdbmodels.TwitterUser{
				ID:        userId,
				Username:  fmt.Sprintf("user-%v", userId),
				AccountID: fmt.Sprintf("user-account-%v", userId),
			}

			err = usr.Insert(common.DB)
			if err != nil {
				return err
			}

		} else {
			return err
		}
	}

	sraw := struct {
		Text string `json:"text,omitempty"`
	}{Text: title}
	raw, err := json.Marshal(sraw)
	if err != nil {
		return err
	}

	mdbTweet := mdbmodels.TwitterTweet{
		ID:        id,
		UserID:    userId,
		TwitterID: tid,
		FullText:  title,
		TweetAt:   time.Now(),
		Raw:       null.NewJSON(raw, true),
		CreatedAt: time.Now(),
	}

	if err := mdbTweet.Insert(common.DB); err != nil {
		return err
	}
	return nil
}

func (suite *IndexerSuite) ucu(cu ContentUnit, lang string, published bool, secure bool) string {
	r := require.New(suite.T())
	uid, err := updateContentUnit(cu, lang, published, secure)
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) setCUType(cu ContentUnit, typeId int64) string {
	r := require.New(suite.T())
	uid, err := setContentUnitType(cu, typeId)
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) uf(f File, cuUID string) string {
	r := require.New(suite.T())
	uid, err := updateFile(f, cuUID)
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) ucut(cu ContentUnit, lang string, tag mdbmodels.Tag, add bool) string {
	r := require.New(suite.T())

	var err error
	var uid string

	if add {
		uid, err = addContentUnitTag(cu, lang, tag)
	} else {
		uid, err = removeContentUnitTag(cu, lang, tag)
	}
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) acus(cu ContentUnit, lang string, src mdbmodels.Source, author mdbmodels.Author, insertAuthor bool) string {
	r := require.New(suite.T())

	var err error
	var uid string

	uid, err = addContentUnitSource(cu, lang, src, author, insertAuthor)
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) rcus(cu ContentUnit, lang string, src mdbmodels.Source) string {
	r := require.New(suite.T())

	var err error
	var uid string

	uid, err = removeContentUnitSource(cu, lang, src)

	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) ucuf(cu ContentUnit, lang string, file mdbmodels.File, add bool) string {
	r := require.New(suite.T())

	var err error
	var uid string

	if add {
		uid, err = addContentUnitFile(cu, lang, file)
	} else {
		uid, err = removeContentUnitFile(cu, lang, file)
	}
	r.Nil(err)
	return uid
}

//update tag
func (suite *IndexerSuite) ut(id int64, parentId null.Int64, name string, lang string) string {
	r := require.New(suite.T())
	uid, err := updateTag(id, parentId, name, lang)
	r.Nil(err)
	return uid
}

func (suite *IndexerSuite) rt(id int64) {
	r := require.New(suite.T())
	r.Nil(removeTag(id))
}

//update source
func (suite *IndexerSuite) us(source Source, lang string) (string, int64) {
	r := require.New(suite.T())
	uid, id, err := updateSource(source, lang)
	r.Nil(err)
	return uid, id
}

//update source file content
func (suite *IndexerSuite) usfc(uid string, lang string) {
	r := require.New(suite.T())
	err := updateSourceFileContent(uid, lang)
	r.Nil(err)
}

//add source author
func (suite *IndexerSuite) asa(source Source, lang string, mdbAuthor mdbmodels.Author, insertAuthor bool, insertI18n bool) {
	r := require.New(suite.T())
	err := addAuthorToSource(source, lang, mdbAuthor, insertAuthor, insertI18n)
	r.Nil(err)
}

//remove source author
func (suite *IndexerSuite) rsa(source Source, mdbAuthor mdbmodels.Author) {
	r := require.New(suite.T())
	err := removeAuthorFromSource(source, mdbAuthor)
	r.Nil(err)
}

//insert blog post
func (suite *IndexerSuite) ibp(wpId int64, blogId int64, title string, filtered bool) string {
	idStr := fmt.Sprintf("%v-%v", blogId, wpId)
	r := require.New(suite.T())
	err := insertPost(wpId, blogId, title, filtered)
	r.Nil(err)
	return idStr
}

//insert tweet
func (suite *IndexerSuite) itt(id int64, tid string, userId int64, title string) {
	r := require.New(suite.T())
	err := insertTweet(id, tid, userId, title)
	r.Nil(err)
}

func (suite *IndexerSuite) validateCollectionsContentUnits(indexName string, indexer *es.Indexer, expectedCUs map[string][]string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	cus := make(map[string][]string)
	for _, hit := range res.Hits.Hits {
		var c Collection
		json.Unmarshal(*hit.Source, &c)
		uids, err := es.KeyValuesToValues("content_unit", c.TypedUIDs)
		r.Nil(err)
		if val, ok := cus[c.MDB_UID]; ok {
			r.Nil(errors.New(fmt.Sprintf(
				"Two identical UID: %s\tFirst : %+v\tSecond: %+v",
				c.MDB_UID, c, val)))
		}
		cus[c.MDB_UID] = uids
	}
	suite.validateMaps(expectedCUs, cus)
}

func (suite *IndexerSuite) validateContentUnitTags(indexName string, indexer *es.Indexer, expectedTags []string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	tags := make([]string, 0)
	for _, hit := range res.Hits.Hits {
		var cu es.Result
		json.Unmarshal(*hit.Source, &cu)
		hitTags, err := es.KeyValuesToValues("tag", cu.FilterValues)
		r.Nil(err)
		tags = append(tags, hitTags...)
	}
	r.Equal(len(expectedTags), len(tags))
	r.ElementsMatch(expectedTags, tags)
}

func (suite *IndexerSuite) validateContentUnitSources(indexName string, indexer *es.Indexer, expectedSources []string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	sources := make([]string, 0)
	for _, hit := range res.Hits.Hits {
		var cu es.Result
		json.Unmarshal(*hit.Source, &cu)
		hitSources, err := es.KeyValuesToValues("source", cu.FilterValues)
		r.Nil(err)
		sources = append(sources, hitSources...)
	}
	r.Equal(len(expectedSources), len(sources))
	r.ElementsMatch(expectedSources, sources)
}

func (suite *IndexerSuite) validateContentUnitFiles(indexName string, indexer *es.Indexer, expectedTranscriptLength null.Int) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)

	// if len(expectedLangs) > 0 {
	// 	langs := make([]string, 0)
	// 	for _, hit := range res.Hits.Hits {
	// 		var cu es.Result
	// 		json.Unmarshal(*hit.Source, &cu)
	// 		for _, t := range cu.Translations {
	// 			langs = append(langs, t)
	// 		}
	// 	}
	//
	// 	r.Equal(len(expectedLangs), len(langs))
	// 	r.ElementsMatch(expectedLangs, langs)
	// }

	// Get transcript,
	transcriptLengths := make([]int, 0)
	for _, hit := range res.Hits.Hits {
		var cu es.Result
		json.Unmarshal(*hit.Source, &cu)
		if cu.Content != "" {
			transcriptLengths = append(transcriptLengths, len(cu.Content))
		}
	}

	if expectedTranscriptLength.Valid {
		r.NotEqual(transcriptLengths, 0)
		r.Contains(transcriptLengths, expectedTranscriptLength.Int)
	} else {
		r.Equal(len(transcriptLengths), 0)
	}
}

func (suite *IndexerSuite) validateMaps(e map[string][]string, a map[string][]string) {
	r := require.New(suite.T())
	for k, v := range e {
		val, ok := a[k]
		r.True(ok, fmt.Sprintf("%s not found in actual: %+v", k, a))
		r.ElementsMatch(v, val, "Elements don't match expected: %+v actual: %+v", v, val)
	}
	for k := range a {
		_, ok := e[k]
		r.True(ok)
	}
}

// Searches content units index and validates their types with expected types.
func (suite *IndexerSuite) validateContentUnitTypes(indexName string, indexer *es.Indexer, expectedTypes map[string][]string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	cus := make(map[string]es.Result)
	for _, hit := range res.Hits.Hits {
		var cu es.Result
		json.Unmarshal(*hit.Source, &cu)
		if val, ok := cus[cu.MDB_UID]; ok {
			r.Nil(errors.New(fmt.Sprintf(
				"Two identical UID: %s\tFirst : %+v\tSecond: %+v",
				cu.MDB_UID, cu, val)))
		}
		cus[cu.MDB_UID] = cu
	}
	types := make(map[string][]string)
	for k, cu := range cus {
		collectionsContentTypes, err := es.KeyValuesToValues("collections_content_type", cu.FilterValues)
		r.Nil(err)
		types[k] = collectionsContentTypes
	}
	suite.validateMaps(expectedTypes, types)

}

func ElementsMatch(r *require.Assertions, a [][]string, b [][]string) {
	r.Equal(len(a), len(b), fmt.Sprintf("%+v is not the same length as %+v", a, b))

	bSlice := b[:]
	for _, arr := range a {
		sort.Strings(arr)
		index := -1
		for i, brr := range bSlice {
			sort.Strings(brr)
			eq := false
			if len(arr) == len(brr) {
				eq = true
				for i := range arr {
					if arr[i] != brr[i] {
						eq = false
						break
					}
				}
			}
			if eq {
				index = i
				break
			}
		}
		r.True(index >= 0, fmt.Sprintf("Did not find %+v of %+v in %+v", arr, a, b))
		bSlice = append(bSlice[:index], bSlice[index+1:]...)
	}

}

func (suite *IndexerSuite) validateTagsFullPath(indexName string, indexer *es.Indexer, expected [][]string) {
	suite.validateFullPath(indexName, indexer, "tag", expected)
}

func (suite *IndexerSuite) validateSourcesFullPath(indexName string, indexer *es.Indexer, expected [][]string) {
	suite.validateFullPath(indexName, indexer, "source", expected)
}

func (suite *IndexerSuite) validateFullPath(indexName string, indexer *es.Indexer, key string, expected [][]string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	sources := make([][]string, 0)
	for _, hit := range res.Hits.Hits {
		var src es.Result
		json.Unmarshal(*hit.Source, &src)
		source := []string{}
		for _, s := range src.FilterValues {
			source = append(source, s)
		}
		formated, err := es.KeyValuesToValues(key, source)
		r.Nil(err)
		sources = append(sources, formated)
	}
	ElementsMatch(r, sources, expected)
}

func (suite *IndexerSuite) validateSourceFile(indexName string, indexer *es.Indexer, expectedContentsByNames map[string]string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	contentsByNames := make(map[string]string)
	for _, hit := range res.Hits.Hits {
		var src es.Result
		json.Unmarshal(*hit.Source, &src)
		contentsByNames[src.MDB_UID] = src.Content
	}

	r.True(reflect.DeepEqual(expectedContentsByNames, contentsByNames))
}

func (suite *IndexerSuite) validateNames(indexName string, indexer *es.Indexer, expectedNames []string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	names := make([]string, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		var res es.Result
		json.Unmarshal(*hit.Source, &res)
		names[i] = res.Title
	}
	r.Equal(int64(len(expectedNames)), res.Hits.TotalHits)
	r.ElementsMatch(names, expectedNames, fmt.Sprintf("Expected names: %+v to be the same as expected names: %+v", names, expectedNames))
}

func (suite *IndexerSuite) validateFullNames(indexName string, indexer *es.Indexer, expectedNames []string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	names := make([]string, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		var res es.Result
		json.Unmarshal(*hit.Source, &res)
		names[i] = res.FullTitle
	}
	r.Equal(int64(len(expectedNames)), res.Hits.TotalHits)
	r.ElementsMatch(names, expectedNames, fmt.Sprintf("Expected names: %+v to be the same as expected names: %+v", names, expectedNames))
}

func (suite *IndexerSuite) validateContents(indexName string, indexer *es.Indexer, expectedContents []string) {
	r := require.New(suite.T())
	err := indexer.RefreshAll()
	r.Nil(err)
	esc, err := common.ESC.GetClient()
	r.Nil(err)
	var res *elastic.SearchResult
	res, err = esc.Search().Index(indexName).Do(suite.ctx)
	r.Nil(err)
	contents := make([]string, len(res.Hits.Hits))
	for i, hit := range res.Hits.Hits {
		var res es.Result
		json.Unmarshal(*hit.Source, &res)
		contents[i] = res.Content
	}
	r.Equal(int64(len(expectedContents)), res.Hits.TotalHits)
	r.ElementsMatch(contents, expectedContents, fmt.Sprintf("Expected contents: %+v to be the same as expected contents: %+v", contents, expectedContents))
}
