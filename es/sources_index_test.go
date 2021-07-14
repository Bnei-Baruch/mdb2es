package es_test

import (
	"fmt"
	"testing"

	"gopkg.in/volatiletech/null.v6"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/Bnei-Baruch/archive-backend/common"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
)

type SourcesIndexerSuite struct {
	IndexerSuite
}

func TestSourcesIndexer(t *testing.T) {
	suite.Run(t, new(SourcesIndexerSuite))
}

func (suite *SourcesIndexerSuite) TestSourcesIndex() {

	es.SetUnzipUrl("test.unzip-url")
	fmt.Printf("\n\n\n--- TEST SOURCES INDEX ---\n\n\n")

	r := require.New(suite.T())

	esc, err := common.ESC.GetClient()
	r.Nil(err)

	indexNameEn := es.IndexName("test", consts.ES_RESULTS_INDEX, consts.LANG_ENGLISH, "test-date")
	indexNameHe := es.IndexName("test", consts.ES_RESULTS_INDEX, consts.LANG_HEBREW, "test-date")
	indexer, err := es.MakeIndexer("test", "test-date", []string{consts.ES_RESULT_TYPE_SOURCES}, common.DB, esc)
	r.Nil(err)

	fmt.Printf("\n\n\nAdding source.\n\n")
	source1UID, source1ID := suite.us(Source{Name: "test-name-1"}, consts.LANG_ENGLISH)
	fileHe := mdbmodels.File{ID: 1, Name: "heb_o_rav_bs-shamati-038-irat-ashem_2016-09-16_lesson.doc", UID: "8awHBfjU", Language: null.String{consts.LANG_HEBREW, true}, Secure: 0, Published: true}
	fileEn := mdbmodels.File{ID: 2, Name: "eng_t_rav_2017-06-09_kitei-makor_mi-ichud-le-hafatza_n1_p1.docx", UID: "frnHYhIw", Language: null.String{consts.LANG_ENGLISH, true}, Secure: 0, Published: true}
	fContentHe, err := es.DocText(fileHe.UID)
	r.Nil(err)
	fContentEn, err := es.DocText(fileEn.UID)
	r.Nil(err)
	suite.ucuf(ContentUnit{Name: "test-name-1", ContentType: consts.CT_SOURCE, UIDForCreate: source1UID}, consts.LANG_HEBREW, fileHe, true)
	suite.ucuf(ContentUnit{MDB_UID: source1UID}, consts.LANG_ENGLISH, fileEn, true)

	suite.us(Source{MDB_UID: source1UID, Name: "שם-בדיקה-1"}, consts.LANG_HEBREW)
	suite.asa(Source{MDB_UID: source1UID}, consts.LANG_ENGLISH, mdbmodels.Author{Name: "Test Name", ID: 3, Code: "t1"}, true, true)
	suite.asa(Source{MDB_UID: source1UID}, consts.LANG_HEBREW, mdbmodels.Author{Name: "שם לבדיקה", ID: 4, Code: "t2"}, true, true)
	fmt.Printf("\n\n\nAdding content files for each language.\n\n")
	suite.usfc(source1UID, consts.LANG_ENGLISH)
	suite.usfc(source1UID, consts.LANG_HEBREW)

	fmt.Printf("\n\n\nReindexing everything.\n\n")

	// Index existing DB data.
	r.Nil(indexer.ReindexAll(esc))
	r.Nil(indexer.RefreshAll())

	r.Nil(es.DumpDB(common.DB, "Before validation"))
	r.Nil(es.DumpIndexes(esc, "Before validation", consts.ES_RESULT_TYPE_SOURCES))

	fmt.Printf("\n\n\nValidate we have source with 2 languages.\n\n")
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1"})
	suite.validateNames(indexNameHe, indexer, []string{"שם-בדיקה-1"})
	suite.validateFullNames(indexNameEn, indexer, []string{"Test Name > test-name-1"})
	suite.validateFullNames(indexNameHe, indexer, []string{"שם לבדיקה > שם-בדיקה-1"})

	fmt.Println("Validate source files.")
	suite.validateSourceFile(indexNameEn, indexer, map[string]string{
		source1UID: fContentEn,
	})
	suite.validateSourceFile(indexNameHe, indexer, map[string]string{
		source1UID: fContentHe,
	})

	fmt.Println("Validate source full path.")
	suite.validateSourcesFullPath(indexNameEn, indexer, [][]string{[]string{source1UID, "t1", "t2"}})

	fmt.Println("Validate adding source without file and author - should not index.")
	source2UID, _ := suite.us(Source{Name: "test-name-2"}, consts.LANG_ENGLISH)
	suite.us(Source{MDB_UID: source2UID, Name: "שם-בדיקה-2"}, consts.LANG_HEBREW)
	r.Nil(indexer.SourceUpdate(source2UID))
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1"})
	suite.validateFullNames(indexNameEn, indexer, []string{"Test Name > test-name-1"})

	fmt.Println("Validate adding source with file but without author - should not index.")
	suite.usfc(source2UID, consts.LANG_ENGLISH)
	suite.usfc(source2UID, consts.LANG_HEBREW)
	r.Nil(indexer.SourceUpdate(source2UID))
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1"})
	suite.validateFullNames(indexNameEn, indexer, []string{"Test Name > test-name-1"})

	fmt.Println("Validate adding source with file and author and and validate.")
	suite.asa(Source{MDB_UID: source2UID}, consts.LANG_ENGLISH, mdbmodels.Author{Name: "Test Name 2", ID: 5, Code: "t3"}, true, true)
	suite.asa(Source{MDB_UID: source2UID}, consts.LANG_HEBREW, mdbmodels.Author{Name: "שם נוסף לבדיקה", ID: 6, Code: "t4"}, true, true)
	fileHe2 := mdbmodels.File{ID: 3, Name: "heb_o_rav_achana_2014-05-28_lesson.doc", UID: "bBBIDCiy", Language: null.String{consts.LANG_HEBREW, true}, Secure: 0, Published: true}
	fileEn2 := mdbmodels.File{ID: 4, Name: "eng_o_rav_2020-12-24_art_on-jewish-unity-no-6.docx", UID: "QrtIVYJA", Language: null.String{consts.LANG_ENGLISH, true}, Secure: 0, Published: true}
	suite.ucuf(ContentUnit{Name: "test-name-2", ContentType: consts.CT_SOURCE, UIDForCreate: source2UID}, consts.LANG_HEBREW, fileHe2, true)
	suite.ucuf(ContentUnit{MDB_UID: source2UID}, consts.LANG_ENGLISH, fileEn2, true)
	r.Nil(indexer.SourceUpdate(source2UID))
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1", "test-name-2"})
	suite.validateFullNames(indexNameEn, indexer, []string{"Test Name > test-name-1", "Test Name 2 > test-name-2"})

	fContentEn2, err := es.DocText(fileEn2.UID)
	r.Nil(err)
	suite.validateSourceFile(indexNameEn, indexer, map[string]string{
		source1UID: fContentEn,
		source2UID: fContentEn2,
	})
	suite.validateSourcesFullPath(indexNameEn, indexer, [][]string{[]string{source1UID, "t1", "t2"}, []string{source2UID, "t3", "t4"}})

	fmt.Printf("\n\n\nAdd source parent like Shamati and validate.\n\n")
	parentShamatiUID, parentShamatiID := suite.us(Source{Name: "Shamati"}, consts.LANG_ENGLISH)
	suite.us(Source{MDB_UID: parentShamatiUID}, consts.LANG_HEBREW)
	consts.ES_SRC_PARENTS_FOR_CHAPTER_POSITION_INDEX[parentShamatiUID] = consts.LETTER_IF_HEBREW
	suite.usfc(parentShamatiUID, consts.LANG_ENGLISH)
	suite.asa(Source{MDB_UID: parentShamatiUID}, consts.LANG_ENGLISH, mdbmodels.Author{Name: "Test Name 2", ID: 7, Code: "t5"}, true, true)
	fileHeShamati := mdbmodels.File{ID: 5, Name: "heb_o_rav_bs-shamati-020-inyan-lishma_2015-07-10_lesson.doc", UID: "72QvKVD8", Language: null.String{consts.LANG_HEBREW, true}, Secure: 0, Published: true}
	fileEnShamati := mdbmodels.File{ID: 6, Name: "eng_t_rav_2010-07-19_program_morim-dereh_anaka-vekesher-rishoni.doc", UID: "yww1AnQ9", Language: null.String{consts.LANG_ENGLISH, true}, Secure: 0, Published: true}
	suite.ucuf(ContentUnit{Name: "test-name-shamati", ContentType: consts.CT_SOURCE, UIDForCreate: parentShamatiUID}, consts.LANG_HEBREW, fileHeShamati, true)
	suite.ucuf(ContentUnit{MDB_UID: parentShamatiUID}, consts.LANG_ENGLISH, fileEnShamati, true)
	r.Nil(indexer.SourceUpdate(parentShamatiUID))
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1", "test-name-2", "Shamati"})
	suite.validateFullNames(indexNameEn, indexer, []string{"Test Name > test-name-1", "Test Name 2 > test-name-2", "Test Name 2 > Shamati"})

	fmt.Printf("Add sources where the position (1) should be indexed as part of the full title (like Shamati chapters).")
	chapterPositionUID, _ := suite.us(Source{Name: "test-name-3",
		ParentID: null.Int64From(parentShamatiID),
		Position: null.IntFrom(1)}, consts.LANG_ENGLISH)
	suite.us(Source{Name: "שם-בדיקה-3", MDB_UID: chapterPositionUID,
		ParentID: null.Int64From(parentShamatiID),
		Position: null.IntFrom(1)}, consts.LANG_HEBREW)
	fileHeChapter := mdbmodels.File{ID: 7, Name: "heb_o_rav_zohar-la-am-ktaim-nivharim_2016-02-17_lesson.doc", UID: "pO6QWIAZ", Language: null.String{consts.LANG_HEBREW, true}, Secure: 0, Published: true}
	fileEnChapter := mdbmodels.File{ID: 8, Name: "eng_t_rav_2020-10-15_lesson_mr-tora-bereshit_n1_p1.docx", UID: "Q7XSgAZy", Language: null.String{consts.LANG_ENGLISH, true}, Secure: 0, Published: true}
	suite.ucuf(ContentUnit{Name: "test-name-chapterPosition", ContentType: consts.CT_SOURCE, UIDForCreate: chapterPositionUID}, consts.LANG_HEBREW, fileHeChapter, true)
	suite.ucuf(ContentUnit{MDB_UID: chapterPositionUID}, consts.LANG_ENGLISH, fileEnChapter, true)
	suite.usfc(chapterPositionUID, consts.LANG_ENGLISH)
	suite.usfc(chapterPositionUID, consts.LANG_HEBREW)

	r.Nil(indexer.SourceUpdate(chapterPositionUID))
	r.Nil(indexer.RefreshAll())

	fmt.Printf("Reindexing everything.")
	r.Nil(indexer.ReindexAll(esc))
	r.Nil(indexer.RefreshAll())

	r.Nil(es.DumpDB(common.DB, "Before validation"))
	r.Nil(es.DumpIndexes(esc, "Before validation", consts.ES_RESULT_TYPE_SOURCES))

	fmt.Printf("Validate we have source with 2 languages and the position is indexed in the full title.")
	suite.validateNames(indexNameEn, indexer, []string{"test-name-1", "test-name-2", "Shamati", "test-name-3"})
	suite.validateNames(indexNameHe, indexer, []string{"שם-בדיקה-2", "שם-בדיקה-1", "שם-בדיקה-3"})
	suite.validateFullNames(indexNameEn, indexer,
		[]string{"Test Name > test-name-1", "Test Name 2 > test-name-2",
			"Test Name 2 > Shamati", "Test Name 2 > Shamati > 1. test-name-3"})
	suite.validateFullNames(indexNameHe, indexer,
		[]string{"שם לבדיקה > שם-בדיקה-1", "שם נוסף לבדיקה > שם-בדיקה-2", "א. שם-בדיקה-3"})

	fmt.Println("Set position to -1 and validate. Should index without position in full title.")
	suite.us(Source{MDB_UID: chapterPositionUID, Position: null.NewInt(-1, true)}, consts.LANG_ENGLISH)
	suite.us(Source{MDB_UID: chapterPositionUID, Position: null.NewInt(-1, true)}, consts.LANG_HEBREW)
	r.Nil(indexer.ReindexAll(esc))
	suite.validateFullNames(indexNameEn, indexer,
		[]string{"Test Name > test-name-1", "Test Name 2 > test-name-2",
			"Test Name 2 > Shamati", "Test Name 2 > Shamati > test-name-3"})
	suite.validateFullNames(indexNameHe, indexer,
		[]string{"שם לבדיקה > שם-בדיקה-1", "שם נוסף לבדיקה > שם-בדיקה-2", "שם-בדיקה-3"})

	fmt.Println("Set position to 244 and validate.")
	suite.us(Source{MDB_UID: chapterPositionUID, Position: null.IntFrom(244)}, consts.LANG_ENGLISH)
	suite.us(Source{MDB_UID: chapterPositionUID, Position: null.IntFrom(244)}, consts.LANG_HEBREW)
	r.Nil(indexer.ReindexAll(esc))
	suite.validateFullNames(indexNameEn, indexer,
		[]string{"Test Name > test-name-1", "Test Name 2 > test-name-2", "Test Name 2 > Shamati", "Test Name 2 > Shamati > 244. test-name-3"})
	suite.validateFullNames(indexNameHe, indexer,
		[]string{"שם לבדיקה > שם-בדיקה-1", "שם נוסף לבדיקה > שם-בדיקה-2", "רמד. שם-בדיקה-3"})

	fmt.Println("Change the parent for 'Shamati chapter' and validate. Should index without position in full title.")
	suite.us(Source{MDB_UID: chapterPositionUID, ParentID: null.Int64From(source1ID)}, consts.LANG_ENGLISH)
	r.Nil(indexer.ReindexAll(esc))
	suite.validateFullNames(indexNameEn, indexer,
		[]string{"Test Name > test-name-1", "Test Name 2 > test-name-2", "Test Name 2 > Shamati", "Test Name > test-name-1 > test-name-3"})
	suite.validateFullNames(indexNameHe, indexer,
		[]string{"שם לבדיקה > שם-בדיקה-1", "שם נוסף לבדיקה > שם-בדיקה-2", "שם לבדיקה > שם-בדיקה-1 > שם-בדיקה-3"})

	fmt.Println("Delete sources from DB, reindex and validate we have 0 sources.")
	suite.rsa(Source{MDB_UID: source1UID}, mdbmodels.Author{ID: 3})
	suite.rsa(Source{MDB_UID: source1UID}, mdbmodels.Author{ID: 4})
	suite.rsa(Source{MDB_UID: source2UID}, mdbmodels.Author{ID: 5})
	suite.rsa(Source{MDB_UID: source2UID}, mdbmodels.Author{ID: 6})
	suite.rsa(Source{MDB_UID: parentShamatiUID}, mdbmodels.Author{ID: 7})

	UIDs := []string{source1UID, source2UID, parentShamatiUID, chapterPositionUID}
	r.Nil(deleteSources(UIDs))
	r.Nil(indexer.ReindexAll(esc))

	suite.validateFullNames(indexNameEn, indexer, []string{})
	suite.validateFullNames(indexNameHe, indexer, []string{})

	// Remove test indexes.
	r.Nil(indexer.DeleteIndexes())
}
