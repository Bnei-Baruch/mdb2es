package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	elastic "gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/bindata"
	"github.com/Bnei-Baruch/archive-backend/common"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Import MDB to ElasticSearch",
	Run:   indexFn,
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

var indexDate string

func init() {
	RootCmd.AddCommand(indexCmd)
	RootCmd.AddCommand(prepareDocsCmd)
	deleteIndexCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to be deleted.")
	deleteIndexCmd.MarkFlagRequired("index_date")
	RootCmd.AddCommand(deleteIndexCmd)
	RootCmd.AddCommand(restartSearchLogsCmd)
	RootCmd.AddCommand(switchAliasCmd)
	RootCmd.AddCommand(updateSynonymsCmd)
	switchAliasCmd.PersistentFlags().StringVar(&indexDate, "index_date", "", "Index date to switch to.")
	switchAliasCmd.MarkFlagRequired("index_date")
}

func indexFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	t := time.Now()
	date := strings.ToLower(t.Format(time.RFC3339))

	err, prevDate := es.ProdAliasedIndexDate(common.ESC)
	if err != nil {
		log.Error(err)
		return
	}

	if date == prevDate {
		log.Info(fmt.Sprintf("New index date is the same as previous index date %s. Wait a minute and rerun.", prevDate))
		return
	}

	indexer, err := es.MakeProdIndexer(date, common.DB, common.ESC)
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
	err = indexer.ReindexAll()
	if err != nil {
		log.Error(err)
		return
	}
	err = es.SwitchProdAliasToCurrentIndex(date, common.ESC)
	if err != nil {
		log.Error(err)
		return
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

	err := es.SwitchProdAliasToCurrentIndex(strings.ToLower(indexDate), common.ESC)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}

func deleteIndexFn(cmd *cobra.Command, args []string) {
	clock := common.Init()
	defer common.Shutdown()

	for _, lang := range consts.ALL_KNOWN_LANGS {
		name := es.IndexName("prod", consts.ES_RESULTS_INDEX, lang, strings.ToLower(indexDate))
		exists, err := common.ESC.IndexExists(name).Do(context.TODO())
		if err != nil {
			log.Error(err)
			return
		}
		if exists {
			res, err := common.ESC.DeleteIndex(name).Do(context.TODO())
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

	name := "search_logs"
	exists, err := common.ESC.IndexExists(name).Do(context.TODO())
	if err != nil {
		log.Error(err)
		return
	}
	if exists {
		res, err := common.ESC.DeleteIndex(name).Do(context.TODO())
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
	res, err := common.ESC.CreateIndex(name).BodyJson(bodyJson).Do(context.TODO())
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

	/*
		Remove this comment later (after reindexing with new assets):
		Do not forget to add analyzer definition for all languages before using synonyms (close the indices, add analyzer and reopen).

		PUT prod_results_[asterisk]/_settings
		{
			"index" : {
				"analysis" : {
					"analyzer" : {
						"synonym" : {
							"tokenizer" : "standard",
							"filter" : ["synonym_graph"]
						}
					}
				}
			}
		}
	*/

	clock := common.Init()
	defer common.Shutdown()

	folder, err := es.SynonymsFolder()
	if err != nil {
		log.Error(errors.Wrap(err, "SynonymsFolder not available."))
		return
	}
	files, err := ioutil.ReadDir(folder)
	if err != nil {
		log.Error(errors.Wrap(err, "Cannot read synonym files list."))
		return
	}

	// TBC add "tokenizer": "keyword"?

	bodyMask := `{
		"index" : {
			"analysis" : {
				"filter" : {
					"%s" : {
						%s
						"tokenizer": "keyword",
						"synonyms" : [
							%s
						]
					}
				}
			}
		}
	}`

	for _, fileInfo := range files {

		keywords := make([]string, 0)
		//autophrases := make([]string, 0)

		//  Convention: file name without extension is the language code.
		var ext = filepath.Ext(fileInfo.Name())
		var lang = fileInfo.Name()[0 : len(fileInfo.Name())-len(ext)]

		indexName := es.IndexNameByDefinedDateOrAlias("prod", consts.ES_RESULTS_INDEX, lang)

		filePath := filepath.Join(folder, fileInfo.Name())
		file, err := os.Open(filePath)
		if err != nil {
			log.Error(errors.Wrapf(err, "Unable to open synonyms file: %s.", filePath))
			return
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()

			//  Blank lines and lines starting with pound are comments (like Solr format).
			if line != "" && !strings.HasPrefix(line, "#") {

				keyphrases := strings.Split(line, ",")
				/*for i := range keyphrases {
					trimmed := strings.TrimSpace(keyphrases[i])
					if strings.Contains(trimmed, " ") {
						autophrase := strings.Replace(trimmed, " ", "_", -1)
						fp := fmt.Sprintf("\"%s => %s\"", trimmed, autophrase)
						autophrases = append(autophrases, fp)
						keyphrases[i] = autophrase
					} else {
						keyphrases[i] = trimmed
					}
					//keyphrases[i] = trimmed
				}*/

				kline := strings.Join(keyphrases, ",")
				fline := fmt.Sprintf("\"%s\"", kline)
				keywords = append(keywords, fline)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Error(errors.Wrapf(err, "Error at scanning synonym config file: %s.", filePath))
			return
		}

		//log.Printf("Keywords: %v", keywords)
		synonymsBody := fmt.Sprintf(bodyMask, "synonym_graph", "\"type\" : \"synonym\",", strings.Join(keywords, ","))

		//log.Printf("Update synonyms request body: %v", body)

		//autophraseBody := fmt.Sprintf(bodyMask, "autophrase_syn", "", strings.Join(autophrases, ","))

		// Close the index in order to update the synonyms
		closeRes, err := common.ESC.CloseIndex(indexName).Do(context.TODO())
		if err != nil {
			log.Error(errors.Wrapf(err, "CloseIndex: %s", indexName))
			return
		}
		if !closeRes.Acknowledged {
			log.Errorf("CloseIndex not Acknowledged: %s", indexName)
			return
		}

		encodedIndexName := url.QueryEscape(indexName)
		_, err = common.ESC.PerformRequest(context.TODO(), elastic.PerformRequestOptions{
			Method: "PUT",
			Path:   fmt.Sprintf("/%s/_settings", encodedIndexName),
			Body:   synonymsBody,
		})
		if err != nil {
			log.Error(errors.Wrapf(err, "Error on updating synonym to elastic index: %s", indexName))
			log.Info("Reopening the index")
			openRes, err := common.ESC.OpenIndex(indexName).Do(context.TODO())
			if err != nil {
				log.Error(errors.Wrapf(err, "OpenIndex: %s", indexName))
			}
			if !openRes.Acknowledged {
				log.Errorf("OpenIndex not Acknowledged: %s", indexName)
			}
			return
		}

		/*_, err = common.ESC.PerformRequest(context.TODO(), elastic.PerformRequestOptions{
			Method: "PUT",
			Path:   fmt.Sprintf("/%s/_settings", encodedIndexName),
			Body:   autophraseBody,
		})
		if err != nil {
			log.Error(errors.Wrapf(err, "Error on updating autophrase_syn to elastic index: %s", indexName))
			log.Info("Reopening the index")
			openRes, err := common.ESC.OpenIndex(indexName).Do(context.TODO())
			if err != nil {
				log.Error(errors.Wrapf(err, "OpenIndex: %s", indexName))
			}
			if !openRes.Acknowledged {
				log.Errorf("OpenIndex not Acknowledged: %s", indexName)
			}
			return
		}*/

		//  Now reopen the index
		openRes, err := common.ESC.OpenIndex(indexName).Do(context.TODO())
		if err != nil {
			log.Error(errors.Wrapf(err, "OpenIndex: %s", indexName))
			return
		}
		if !openRes.Acknowledged {
			log.Errorf("OpenIndex not Acknowledged: %s", indexName)
			return
		}

	}

	log.Info("Success")
	log.Infof("Total run time: %s", time.Now().Sub(clock).String())
}
