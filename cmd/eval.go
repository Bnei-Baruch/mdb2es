package cmd

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/search"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate recall set.",
	Run:   evalFn,
}

var evalDiffCmd = &cobra.Command{
	Use:   "eval_diff",
	Short: "Evaluate diff.",
	Run:   evalDiffFn,
}

var vsGoldenHtmlCmd = &cobra.Command{
	Use:   "vs_golden_html",
	Short: "Compares full reports and generates comparison HTML.",
	Run:   vsGoldenHtmlFn,
}

var testTypoSuggestCmd = &cobra.Command{
	Use:   "test_typo_suggest",
	Short: "Test typo suggest for a list of search queries.",
	Run:   testTypoSuggestFn,
}

var evalSetPath string
var serverUrl string
var baseServerUrl string
var clientUrl string
var clientBaseUrl string
var reportPath string
var flatReportPath string
var flatReportsPaths string
var goldenFlatReportPaths string
var vsGoldenHtml string
var EvalScrape string
var evalDiffHtml string
var top int
var typosPath string
var language string
var htmlFileToInject string

func init() {
	evalCmd.PersistentFlags().StringVar(&evalSetPath, "eval_set", "", "Path to csv eval set.")
	evalCmd.MarkFlagRequired("eval_set")
	evalCmd.PersistentFlags().StringVar(&reportPath, "report", "", "Path to csv report file per query.")
	evalCmd.MarkFlagRequired("report")
	evalCmd.PersistentFlags().StringVar(&flatReportPath, "flat_report", "", "Path to csv report file per expectation.")
	evalCmd.MarkFlagRequired("flat_report")
	evalCmd.PersistentFlags().StringVar(&serverUrl, "server", "", "URL of experimental archive backend to evaluate.")
	evalCmd.MarkFlagRequired("server")
	evalCmd.PersistentFlags().StringVar(&baseServerUrl, "base_server", "", "URL of base archive backend to evaluate.")
	evalCmd.MarkFlagRequired("base_server")
	RootCmd.AddCommand(evalCmd)

	evalDiffCmd.PersistentFlags().StringVar(&evalSetPath, "eval_set", "", "Path to csv eval set.")
	evalDiffCmd.MarkFlagRequired("eval_set")
	evalDiffCmd.PersistentFlags().StringVar(&evalDiffHtml, "eval_diff_html", "", "Path to html with eval diff results.")
	evalDiffCmd.MarkFlagRequired("eval_diff_html")
	evalDiffCmd.PersistentFlags().StringVar(&serverUrl, "server", "", "URL of experimental archive backend to evaluate.")
	evalDiffCmd.MarkFlagRequired("server")
	evalDiffCmd.PersistentFlags().StringVar(&baseServerUrl, "base_server", "", "URL of base archive backend to evaluate.")
	evalDiffCmd.MarkFlagRequired("base_server")
	evalDiffCmd.PersistentFlags().IntVar(&top, "top", 0, "Limit query set size.")
	evalDiffCmd.PersistentFlags().StringVar(&clientUrl, "client", "", "URL of experimental archive client to evaluate.")
	evalDiffCmd.PersistentFlags().StringVar(&clientBaseUrl, "client_base", "", "URL of base archive client to evaluate.")
	RootCmd.AddCommand(evalDiffCmd)

	vsGoldenHtmlCmd.PersistentFlags().StringVar(&flatReportsPaths, "flat_reports", "", "Paths to csv report file per expectation, separated by comma.")
	vsGoldenHtmlCmd.MarkPersistentFlagRequired("flat_reports")
	vsGoldenHtmlCmd.PersistentFlags().StringVar(&goldenFlatReportPaths, "golden_flat_reports", "", "Paths to csv golden report file per expectation, separated by comma.")
	vsGoldenHtmlCmd.MarkPersistentFlagRequired("golden_flat_reports")
	vsGoldenHtmlCmd.PersistentFlags().StringVar(&vsGoldenHtml, "vs_golden_html", "", "Path to html output of comparison vs golden.")
	vsGoldenHtmlCmd.MarkPersistentFlagRequired("vs_golden_html")
	vsGoldenHtmlCmd.PersistentFlags().StringVar(&htmlFileToInject, "html_to_inject", "", "Optional HTML file to put his content at the buttom of the output HTML.")
	RootCmd.AddCommand(vsGoldenHtmlCmd)

	testTypoSuggestCmd.PersistentFlags().StringVar(&evalSetPath, "eval_set", "", "Path to csv eval set.")
	testTypoSuggestCmd.MarkFlagRequired("eval_set")
	testTypoSuggestCmd.PersistentFlags().StringVar(&typosPath, "typos_path", "", "Path to typos list file.")
	testTypoSuggestCmd.MarkFlagRequired("typos_path")
	testTypoSuggestCmd.PersistentFlags().StringVar(&language, "lang", "", "Index language.")
	testTypoSuggestCmd.MarkFlagRequired("lang")
	RootCmd.AddCommand(testTypoSuggestCmd)
}

func roundD(val float64) int {
	if val < 0 {
		return int(val - 1.0)
	}
	return int(val)
}

func float64ToPercent(val float64) string {
	return fmt.Sprintf("%.2f%%", float64(roundD(val*10000))/float64(100))
}

func runExpVsBase(evalSet []search.EvalQuery, baseUrl string, expUrl string) (
	search.EvalResults, map[int][]search.Loss, search.EvalResults, map[int][]search.Loss, error) {
	var wg sync.WaitGroup
	wg.Add(2)

	var baseResults search.EvalResults
	var baseLosses map[int][]search.Loss
	var baseErr error
	go func() {
		defer wg.Done()
		baseResults, baseLosses, baseErr = search.Eval(evalSet, baseUrl)
	}()

	var expResults search.EvalResults
	var expLosses map[int][]search.Loss
	var expErr error
	go func() {
		defer wg.Done()
		expResults, expLosses, expErr = search.Eval(evalSet, expUrl)
	}()
	wg.Wait()
	if baseErr != nil {
		return search.EvalResults{}, nil, search.EvalResults{}, nil, baseErr
	}
	if expErr != nil {
		return search.EvalResults{}, nil, search.EvalResults{}, nil, expErr
	}
	return baseResults, baseLosses, expResults, expLosses, nil
}

func printResults(results search.EvalResults) {
	log.Infof("Unique queries: %d", results.TotalUnique)
	log.Infof("Weighted queries: %f", results.TotalWeighted)
	log.Infof("Errors: %d", results.TotalErrors)
	var keys []int
	for k, _ := range results.UniqueMap {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		unique := results.UniqueMap[k]
		weighted := results.WeightedMap[k]
		log.Infof("%-15s Unique/Weighted: %7s/%7s", search.SEARCH_QUALITY_NAME[k], float64ToPercent(unique), float64ToPercent(weighted))
	}
}

func printLosses(results search.EvalResults, losses map[int][]search.Loss) {
	log.Infof("Found %d loss types.", len(losses))
	var lKeys []int
	for k, _ := range losses {
		lKeys = append(lKeys, k)
	}
	sort.Ints(lKeys)
	for _, eType := range lKeys {
		lList := losses[eType]
		totalUnique := float64(0)
		totalWeighted := float64(0)
		for _, l := range lList {
			totalUnique += l.Unique
			totalWeighted += l.Weighted
		}
		log.Infof("%s - %7s/%7s ", search.EXPECTATION_HIT_TYPE[eType],
			float64ToPercent(totalUnique/float64(results.TotalUnique)),
			float64ToPercent(totalWeighted/float64(results.TotalWeighted)))
		for _, l := range lList {
			log.Infof("\t%7s/%7s - Query: [%s] Bucket: %s %+v", float64ToPercent(l.Unique/float64(results.TotalUnique)),
				float64ToPercent(l.Weighted/float64(results.TotalWeighted)), l.Query.Query, l.Query.Bucket, l.Expectation)
		}
	}
}

func Round(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}

func evalDiffFn(cmd *cobra.Command, args []string) {
	log.Infof("Evaluating diff set at %s.", evalSetPath)
	evalSet, err := search.ReadEvalDiffSet(evalSetPath)
	utils.Must(err)
	if top > 0 {
		sort.SliceStable(evalSet, func(i, j int) bool {
			return evalSet[i].Weight > evalSet[j].Weight
		})
		evalSet = evalSet[:utils.MinInt(top, len(evalSet))]
	}

	if evalDiffHtml == "" {
		panic("eval_diff_html must be set.")
	}

	diffs, err := search.EvalQuerySetDiff(evalSet, baseServerUrl, serverUrl, -1 /*diffsLimit*/)
	utils.Must(err)
	// Generate eval diff html report.
	html, err := search.EvalResultsDiffsHtml(diffs, clientUrl, clientBaseUrl)
	utils.Must(err)
	utils.Must(ioutil.WriteFile(evalDiffHtml, []byte(html), 0644))
}

func evalFn(cmd *cobra.Command, args []string) {
	log.Infof("Evaluating eval set at %s.", evalSetPath)
	evalSet, err := search.InitAndReadEvalSet(evalSetPath)
	utils.Must(err)
	if baseServerUrl != "" {
		baseResults, baseLosses, expResults, expLosses, err := runExpVsBase(evalSet, baseServerUrl, serverUrl)
		utils.Must(err)
		log.Infof("Base:")
		printResults(baseResults)
		log.Infof("Exp:")
		printResults(expResults)
		log.Infof("Base:")
		printLosses(baseResults, baseLosses)
		log.Infof("Exp:")
		printLosses(expResults, expLosses)
		if len(baseResults.Results) != len(expResults.Results) {
			log.Errorf("Expected same number of results for exp and base, got base - %d and exp - %d.",
				len(baseResults.Results), len(expResults.Results))
			return
		}
		classification := make(map[int][]string)
		for i, baseResult := range baseResults.Results {
			expResult := expResults.Results[i]
			if len(baseResult.SearchQuality) != len(expResult.SearchQuality) {
				log.Errorf("Expected same number of expectations (search quality) for base and exp, got base - %d and exp - %d",
					len(baseResult.SearchQuality), len(expResult.SearchQuality))
				return
			}
			for j, baseSQ := range baseResult.SearchQuality {
				expSQ := expResult.SearchQuality[j]
				cr := search.CompareResults(baseSQ, expSQ)
				queryWeight := Round(float64(evalSet[i].Weight))
				expectationWeight := Round(float64(queryWeight) / float64(len(evalSet[i].Expectations)))
				expectationUniqueWeight := Round(1 / float64(len(evalSet[i].Expectations)))
				expectation := evalSet[i].Expectations[j]
				baseRank := baseResult.Rank[j]
				expRank := expResult.Rank[j]
				str := fmt.Sprintf("\t%.2f/%.2f   %.2f/%.2f - [%s] - Expectation: (type: %s, uid: %s) Rank: (base: %d  exp: %d)",
					expectationUniqueWeight, 1.0, expectationWeight, queryWeight, evalSet[i].Query,
					search.EXPECTATION_HIT_TYPE[expectation.Type], expectation.Uid, baseRank, expRank)
				classification[cr] = append(classification[cr], str)
			}
		}

		var keys []int
		for k, _ := range classification {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		for _, k := range keys {
			if k != search.CR_SAME {
				log.Infof("%s:", search.COMPARE_RESULTS_NAME[k])
				for _, str := range classification[k] {
					log.Info(str)
				}
			}
		}
		search.WriteResults(reportPath, evalSet, expResults)
		search.WriteResultsByExpectation(flatReportPath, evalSet, expResults)
	} else {
		results, losses, err := search.Eval(evalSet, serverUrl)
		utils.Must(err)
		printResults(results)
		printLosses(results, losses)
		if len(reportPath) == 0 {
			log.Warn("Cannot write results: reportPath is not set!", reportPath)
		} else {
			err = search.WriteResults(reportPath, evalSet, results)
			utils.Must(err)
		}
		if len(flatReportPath) == 0 {
			log.Warn("Cannot write result by expectation: flatReportPath is not set!", flatReportPath)
		} else {
			_, err = search.WriteResultsByExpectation(flatReportPath, evalSet, results)
			utils.Must(err)
		}
	}
	utils.Must(err)
	log.Infof("Done evaluating queries.")
}

func vsGoldenHtmlFn(cmd *cobra.Command, args []string) {
	allRecords := [][]string{}
	for _, path := range strings.Split(flatReportsPaths, ",") {
		log.Infof("Opening: %s", path)
		reader, err := os.Open(path)
		r := csv.NewReader(bufio.NewReader(reader))
		records, err := r.ReadAll()
		utils.Must(err)
		allRecords = append(allRecords, records[1:]...)
	}
	allGoldenRecords := [][]string{}
	for _, path := range strings.Split(goldenFlatReportPaths, ",") {
		log.Infof("Opening: %s", path)
		reader, err := os.Open(path)
		r := csv.NewReader(bufio.NewReader(reader))
		recordsGolden, err := r.ReadAll()
		utils.Must(err)
		allGoldenRecords = append(allGoldenRecords, recordsGolden[1:]...)
	}
	var buttomPart string
	if htmlFileToInject != "" {
		b, err := ioutil.ReadFile(htmlFileToInject)
		if err != nil {
			log.Error(err)
		} else {
			buttomPart = string(b)
		}
	}
	if err := search.WriteVsGoldenHTML(vsGoldenHtml, allRecords, allGoldenRecords, buttomPart); err != nil {
		log.Error(err)
	}
}

func testTypoSuggestFn(cmd *cobra.Command, args []string) {
	esManager := search.MakeESManager(elasticUrl)
	esc, err := esManager.GetClient()
	utils.Must(err)
	engine := search.NewESEngine(esc, nil, nil, nil, nil)

	evalSet, err := search.InitAndReadEvalSet(evalSetPath)
	utils.Must(err)

	file, err := os.Open(typosPath)
	utils.Must(err)
	defer file.Close()

	typos := make([]string, 0)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		typos = append(typos, scanner.Text())
	}
	utils.Must(scanner.Err())

	falsePositiveCnt := 0
	noSuggestForTypoCnt := 0

	log.Info("*** CHECKING KNOWN TYPOS ***")

	for _, t := range typos {
		query := search.Query{Term: t, LanguageOrder: consts.SEARCH_LANG_ORDER[language]}
		res, err := engine.GetTypoSuggest(query)
		utils.Must(err)
		if res.Valid {
			log.Infof("Suggest for '%s' is: '%s'.", t, res.String)
		} else {
			noSuggestForTypoCnt++
			log.Infof("No suggest for '%s'!", t)
		}
	}

	log.Info("\n*** CHECKING FALSE POSITIVE FROM RECALL SET***")

	for _, e := range evalSet {

		query := search.Query{Term: e.Query, LanguageOrder: consts.SEARCH_LANG_ORDER[language]}

		res, err := engine.GetTypoSuggest(query)
		utils.Must(err)
		if res.Valid {
			log.Infof("Suggest for '%s' is: '%s'. Check if this is false positive.", e.Query, res.String)
			falsePositiveCnt++
		} else {
			log.Infof("No suggest for '%s'.", e.Query)
		}
	}

	log.Infof("\n\nNot found suggests for known typos: %d from %d.\nProbable False Positive: %d from %d.", noSuggestForTypoCnt, len(typos), falsePositiveCnt, len(evalSet))
}
