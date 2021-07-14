package es_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/volatiletech/null.v6"

	"github.com/Bnei-Baruch/archive-backend/common"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/Bnei-Baruch/archive-backend/mdb/models"
)

type LikutimIndexerSuite struct {
	IndexerSuite
}

func TestLikutimIndexer(t *testing.T) {
	suite.Run(t, new(LikutimIndexerSuite))
}

func (suite *LikutimIndexerSuite) TestLikutimIndex() {
	fmt.Printf("\n\n\n--- TEST Likutim INDEX ---\n\n\n")

	r := require.New(suite.T())

	esc, err := common.ESC.GetClient()
	r.Nil(err)

	fmt.Printf("\n\n\nAdding content units.\n\n")
	cu1UID := suite.ucu(ContentUnit{Name: "something", ContentType: consts.CT_LIKUTIM}, consts.LANG_ENGLISH, true, true)
	suite.ucu(ContentUnit{MDB_UID: cu1UID, Name: "משהוא"}, consts.LANG_HEBREW, true, true)
	suite.ucu(ContentUnit{MDB_UID: cu1UID, Name: "чтото"}, consts.LANG_RUSSIAN, true, true)
	cu2UID := suite.ucu(ContentUnit{Name: "something else", ContentType: consts.CT_LIKUTIM}, consts.LANG_ENGLISH, true, true)
	cuNotLikutimUID := suite.ucu(ContentUnit{Name: "not likutim", ContentType: consts.CT_LESSON_PART}, consts.LANG_ENGLISH, true, true)
	cuNotPublishedUID := suite.ucu(ContentUnit{Name: "not published"}, consts.LANG_ENGLISH, false, true)
	cuNotSecureUID := suite.ucu(ContentUnit{Name: "not secured"}, consts.LANG_ENGLISH, true, false)
	UIDs := []string{cu1UID, cu2UID, cuNotLikutimUID, cuNotPublishedUID, cuNotSecureUID}

	fmt.Printf("\n\n\nReindexing everything.\n\n")
	indexNameEn := es.IndexName("test", consts.ES_RESULTS_INDEX, consts.LANG_ENGLISH, "test-date")
	indexNameHe := es.IndexName("test", consts.ES_RESULTS_INDEX, consts.LANG_HEBREW, "test-date")
	indexNameRu := es.IndexName("test", consts.ES_RESULTS_INDEX, consts.LANG_RUSSIAN, "test-date")
	indexer, err := es.MakeIndexer("test", "test-date", []string{consts.ES_RESULT_TYPE_LIKUTIM}, common.DB, esc)
	r.Nil(err)

	// Index existing DB data.
	r.Nil(indexer.ReindexAll(esc))
	r.Nil(indexer.RefreshAll())

	fmt.Println("Validate we have 2 searchable content units.")
	suite.validateNames(indexNameEn, indexer, []string{"something", "something else"})

	fmt.Println("Add a file to content unit and validate.")
	transcriptContent := "1234"
	suite.serverResponses["/doc2text/dEvgPVpr"] = transcriptContent
	file := mdbmodels.File{ID: 1, Name: "heb_o_rav_2017-05-25_lesson_achana_n1_p0.doc", UID: "dEvgPVpr", Language: null.String{"he", true}, Secure: 0, Published: true}
	f1UID := suite.ucuf(ContentUnit{MDB_UID: cu1UID}, consts.LANG_HEBREW, file, true)
	r.Nil(indexer.FileUpdate(f1UID))
	suite.validateNames(indexNameEn, indexer, []string{"something", "something else"})
	suite.validateContentUnitFiles(indexNameHe, indexer, null.Int{len(transcriptContent), true})
	fmt.Println("Remove a file from content unit and validate.")
	suite.ucuf(ContentUnit{MDB_UID: cu1UID}, consts.LANG_HEBREW, file, false)
	r.Nil(indexer.FileUpdate(f1UID))
	r.Nil(es.DumpDB(common.DB, "DumpDB"))
	r.Nil(es.DumpIndexes(esc, "DumpIndexes", consts.ES_RESULT_TYPE_LIKUTIM))
	suite.validateContentUnitFiles(indexNameHe, indexer, null.Int{-1, false})

	fmt.Println("Add a tag to content unit and validate.")
	suite.ucut(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, mdbmodels.Tag{Pattern: null.String{"ibur", true}, ID: 1, UID: "L2jMWyce"}, true)
	r.Nil(indexer.ContentUnitUpdate(cu1UID))
	suite.validateContentUnitTags(indexNameEn, indexer, []string{"L2jMWyce"})
	fmt.Println("Add second tag to content unit and validate.")
	suite.ucut(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, mdbmodels.Tag{Pattern: null.String{"arvut", true}, ID: 2, UID: "L3jMWyce"}, true)
	r.Nil(indexer.ContentUnitUpdate(cu1UID))
	suite.validateContentUnitTags(indexNameEn, indexer, []string{"L2jMWyce", "L3jMWyce"})
	fmt.Println("Remove one tag from content unit and validate.")
	suite.ucut(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, mdbmodels.Tag{Pattern: null.String{"ibur", true}, ID: 1, UID: "L2jMWyce"}, false)
	r.Nil(indexer.ContentUnitUpdate(cu1UID))
	suite.validateContentUnitTags(indexNameEn, indexer, []string{"L3jMWyce"})
	fmt.Println("Remove the second tag.")
	suite.ucut(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, mdbmodels.Tag{Pattern: null.String{"arvut", true}, ID: 2, UID: "L3jMWyce"}, false)

	fmt.Println("Make content unit not published and validate.")
	//r.Nil(es.DumpDB(common.DB, "TestContentUnitsIndex, BeforeDB"))
	//r.Nil(es.DumpIndexes(common.ESC, "TestContentUnitsIndex, BeforeIndexes", consts.ES_RESULT_TYPE_UNITS))
	suite.ucu(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, false, true)
	r.Nil(indexer.ContentUnitUpdate(cu1UID))
	//r.Nil(es.DumpDB(common.DB, "TestContentUnitsIndex, AfterDB"))
	//r.Nil(es.DumpIndexes(common.ESC, "TestContentUnitsIndex, AfterIndexes", consts.ES_RESULT_TYPE_UNITS))
	suite.validateNames(indexNameEn, indexer, []string{"something else"})
	suite.validateNames(indexNameHe, indexer, []string{})
	suite.validateNames(indexNameRu, indexer, []string{})

	fmt.Println("Make content unit not secured and validate.")
	suite.ucu(ContentUnit{MDB_UID: cu2UID}, consts.LANG_ENGLISH, true, false)
	r.Nil(indexer.ContentUnitUpdate(cu2UID))
	suite.validateNames(indexNameEn, indexer, []string{})
	suite.validateNames(indexNameHe, indexer, []string{})
	suite.validateNames(indexNameRu, indexer, []string{})

	fmt.Println("Secure and publish content units again and check we have 2 searchable content units.")
	suite.ucu(ContentUnit{MDB_UID: cu1UID}, consts.LANG_ENGLISH, true, true)
	r.Nil(indexer.ContentUnitUpdate(cu1UID))
	suite.ucu(ContentUnit{MDB_UID: cu2UID}, consts.LANG_ENGLISH, true, true)
	r.Nil(indexer.ContentUnitUpdate(cu2UID))
	suite.validateNames(indexNameEn, indexer, []string{"something", "something else"})
	suite.validateNames(indexNameHe, indexer, []string{"משהוא"})
	suite.validateNames(indexNameRu, indexer, []string{"чтото"})

	fmt.Println("Validate adding content unit incrementally.")
	var cu3UID string
	cu3UID = suite.ucu(ContentUnit{Name: "third something"}, consts.LANG_ENGLISH, true, true)
	UIDs = append(UIDs, cu3UID)
	r.Nil(indexer.ContentUnitUpdate(cu3UID))
	suite.validateNames(indexNameEn, indexer,
		[]string{"something", "something else", "third something"})

	fmt.Println("Update content unit and validate.")
	suite.ucu(ContentUnit{MDB_UID: cu3UID, Name: "updated third something"}, consts.LANG_ENGLISH, true, true)
	r.Nil(indexer.ContentUnitUpdate(cu3UID))
	suite.validateNames(indexNameEn, indexer,
		[]string{"something", "something else", "updated third something"})

	fmt.Println("Delete content unit and validate nothing changes as the database did not change!")
	r.Nil(indexer.ContentUnitUpdate(cu2UID))
	suite.validateNames(indexNameEn, indexer, []string{"something", "something else", "updated third something"})

	fmt.Println("Now actually delete the content unit also from database.")
	r.Nil(deleteContentUnits([]string{cu2UID}))
	r.Nil(indexer.ContentUnitUpdate(cu2UID))
	suite.validateNames(indexNameEn, indexer, []string{"something", "updated third something"})

	fmt.Println("Delete units, reindex and validate we have 0 searchable units.")
	r.Nil(deleteContentUnits(UIDs))
	r.Nil(indexer.ReindexAll(esc))
	suite.validateNames(indexNameEn, indexer, []string{})

	//fmt.Println("Restore docx-folder path to original.")
	//mdb.DocFolder = originalDocxPath

	// Remove test indexes.
	r.Nil(indexer.DeleteIndexes())
}
