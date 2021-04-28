package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/Bnei-Baruch/archive-backend/bindata"
	"github.com/Bnei-Baruch/archive-backend/common"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/Bnei-Baruch/archive-backend/search"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Import MDB to ElasticSearch",
	Run:   indexFn,
}

var indexGrammarsCmd = &cobra.Command{
	Use:   "index_grammars",
	Short: "Import Grammars to ElasticSearch",
	Run:   indexGrammarsFn,
}

var prepareDocsCmd = &cobra.Command{
	Use:   "prepare_docs",
	Short: "Prepares all docs via Unzip service.",
	Run:   prepareDocsFn,
}

var deleteIndexCmd = &cobra.Command{
	Use:   "delete_index",
	Short: "Delete index.",
	Run:   deleteIndexFn,
}

var deleteGrammarIndexCmd = &cobra.Command{
	Use:   "delete_grammar_index",
	Short: "Delete grammar index.",
	Run:   deleteGrammarIndexFn,
}

var restartSearchLogsCmd = &cobra.Command{
	Use:   "restart_search_logs",
	Short: "Restarts search logs.",
	Run:   restartSearchLogsFn,
}

var switchAliasCmd = &cobra.Command{
	Use:   "switch_alias",
	Short: "Switch Elastic to use different index.",
	Run:   switchAliasFn,
}

var updateSynonymsCmd = &cobra.Command{
	Use:   "update_synonyms",
	Short: "Update synonym keywords list.",
	Run:   updateSynonymsFn,
}

var simulateUpdateCmd = &cobra.Command{
	Use:   "simulate_update",
	Short: "Simulate index update.",
	Run:   simulateUpdateFn,
}

var indexDate string
var updateAlias bool

func init() {
	RootCmd.AddCommand(indexCmd)
	indexCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be used for new index.")
	indexCmd.PersistentFlags().BoolVar(&updateAlias, "update_alias", true, "If set to false will not update alias.")
	RootCmd.AddCommand(indexGrammarsCmd)
	indexGrammarsCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be used for new index.")
	indexGrammarsCmd.PersistentFlags().BoolVar(&updateAlias, "update_alias", true, "If set to false will not update alias.")
	RootCmd.AddCommand(prepareDocsCmd)
	deleteIndexCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be deleted.")
	deleteIndexCmd.MarkFlagRequired("index_date")
	deleteGrammarIndexCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be deleted.")
	deleteGrammarIndexCmd.MarkFlagRequired("index_date")
	RootCmd.AddCommand(deleteIndexCmd)
	RootCmd.AddCommand(deleteGrammarIndexCmd)
	RootCmd.AddCommand(restartSearchLogsCmd)
	RootCmd.AddCommand(updateSynonymsCmd)
	updateSynonymsCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be deleted.")
	updateSynonymsCmd.MarkFlagRequired("index_date")
	RootCmd.AddCommand(switchAliasCmd)
	switchAliasCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to switch to.")
	switchAliasCmd.MarkFlagRequired("index_date")
	RootCmd.AddCommand(simulateUpdateCmd)
}

func indexGrammarsFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()
	log.Infof("Initialized.")

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	date := getDateAlias()

	if indexDate != "" {
		date = indexDate
	}

	alias := search.GrammarIndexName("%s", "")
	aliasRegexp := search.GrammarIndexName(".*", ".*")
	err, prev := es.AliasedIndex(esc, alias, aliasRegexp)
	utils.Must(err)
	if date == prev {
		log.Info(fmt.Sprintf("New index date is the same as previous index date %s. Wait a minute and rerun.", prev))
		return
	}
	if prev != "" {
		prev = search.GrammarIndexName("%s", prev)
	}

	log.Infof("Client loaded.")
	variables, err := search.MakeVariablesV2(viper.GetString("elasticsearch.variables"))
	utils.Must(err)
	log.Infof("Variables loaded.")
	grammars, err := search.MakeGrammarsV2(viper.GetString("elasticsearch.grammars"))
	utils.Must(err)
	log.Infof("Grammas loaded.")

	err = search.IndexGrammars(esc, date, grammars, variables, common.CACHE)
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
	} else {
		log.Info("Grammar indexed.")
	}

	if updateAlias {
		utils.Must(es.SwitchAlias(alias, prev, search.GrammarIndexName("%s", date), esc))
	} else {
		log.Info("Not switching alias.")
	}

	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func indexFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	date := getDateAlias()

	if indexDate != "" {
		date = indexDate
	}

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	err, prevDate := es.ProdIndexDate(esc)
	if err != nil {
		log.Error(err)
		return
	}

	// Check that we did not set specifi index, otherwise we will always have "same date".
	indexDate := viper.GetString("elasticsearch.index-date")
	if indexDate == "" && date == prevDate {
		log.Info(fmt.Sprintf("New index date is the same as previous index date %s. Wait a minute and rerun.", prevDate))
		return
	}

	indexer, err := es.MakeProdIndexer(date, common.DB, esc)
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("Preparing all documents with Unzip.")
	err = es.ConvertDocx(common.DB)
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("Done preparing documents.")
	err = indexer.ReindexAll(esc)
	if err != nil {
		log.Error(err)
		return
	}

	if updateAlias {
		err = es.SwitchProdAliasToCurrentIndex(date, esc)
		if err != nil {
			log.Error(err)
			return
		}
	} else {
		log.Info("Not switching alias.")
	}
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func prepareDocsFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	log.Info("Preparing all documents with Unzip.")
	err := es.ConvertDocx(common.DB)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info("Done preparing documents.")
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func switchAliasFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	err = es.SwitchProdAliasToCurrentIndex(strings.ToLower(indexDate), esc)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func deleteIndexFn(cmd *cobra.Command, args []string) {
	deleteIndex(cmd, args, es.IndexNameFuncByNamespaceAndDate("prod", strings.ToLower(indexDate)))
}

func deleteGrammarIndexFn(cmd *cobra.Command, args []string) {
	deleteIndex(cmd, args, search.GrammarIndexNameFunc(strings.ToLower(indexDate)))
}

func deleteIndex(cmd *cobra.Command, args []string, indexByLang es.IndexNameByLang) {
	clock := common.Init()
	defer common.Shutdown()

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	for _, lang := range consts.ALL_KNOWN_LANGS {
		name := indexByLang(lang)
		exists, err := esc.IndexExists(name).Do(context.TODO())
		if err != nil {
			log.Error(err)
			return
		}
		if exists {
			res, err := esc.DeleteIndex(name).Do(context.TODO())
			if err != nil {
				log.Error(errors.Wrap(err, "Delete index"))
				return
			}
			if !res.Acknowledged {
				log.Error(errors.Errorf("Index deletion wasn't acknowledged: %s", name))
				return
			}
		}
	}
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func restartSearchLogsFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	name := "search_logs"
	exists, err := esc.IndexExists(name).Do(context.TODO())
	if err != nil {
		log.Error(err)
		return
	}
	if exists {
		res, err := esc.DeleteIndex(name).Do(context.TODO())
		if err != nil {
			log.Error(errors.Wrap(err, "Delete index"))
			return
		}
		if !res.Acknowledged {
			log.Error(errors.Errorf("Index deletion wasn't acknowledged: %s", name))
			return
		}
	}

	definition := fmt.Sprintf("data/es/mappings/%s.json", name)
	// Read mappings and create index
	mappings, err := bindata.Asset(definition)
	if err != nil {
		log.Error(errors.Wrapf(err, "Failed loading mapping %s", definition))
		return
	}
	var bodyJson map[string]interface{}
	if err = json.Unmarshal(mappings, &bodyJson); err != nil {
		log.Error(errors.Wrap(err, "json.Unmarshal"))
		return
	}
	// Create index.
	res, err := esc.CreateIndex(name).BodyJson(bodyJson).Do(context.TODO())
	if err != nil {
		log.Error(errors.Wrap(err, "Create index"))
		return
	}
	if !res.Acknowledged {
		log.Error(errors.Errorf("Index creation wasn't acknowledged: %s", name))
		return
	}
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func updateSynonymsFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	esc, err := common.ESC.GetClient()
	if err != nil {
		log.Error(errors.Wrap(err, "Failed to connect to ElasticSearch."))
		return
	}

	// Update synonyms.
	err = es.UpdateSynonyms(esc, es.IndexNameFuncByNamespaceAndDate("prod", indexDate))
	if err != nil {
		log.Error(err)
		return
	}

	// Update grammar synonyms.
	err = es.UpdateSynonyms(esc, search.GrammarIndexNameFunc(indexDate))
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func simulateUpdateFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	client, err := common.ESC.GetClient()
	if err != nil {
		log.Error(err)
		return
	}

	err, date := es.ProdIndexDate(client)
	if err != nil {
		log.Error(err)
		return
	}

	indexer, err := es.MakeProdIndexer(date, common.DB, client)
	if err != nil {
		log.Error(err)
		return
	}

	//err = indexer.Update(es.Scope{CollectionUID: "zf4lLwyI"})
	err = indexer.Update(es.Scope{SourceUID: "qMUUn22b"})
	//err = indexer.Update(es.Scope{ContentUnitUID: "S5cSiwqb"})
	//err = indexer.Update(es.Scope{FileUID: "QSMWk1lj"})
	//err = indexer.Update(es.Scope{ContentUnitUID: "eA0g9XRf"})
	if err != nil {
		log.Error(err)
		return
	}

	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func getDateAlias() string {
	t := time.Now()
	date := strings.ToLower(t.Format(time.RFC3339))
	return strings.ReplaceAll(date, "+", "p")
}
