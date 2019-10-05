package cmd

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/volatiletech/null.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"

	log "github.com/Sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/search"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "log commands",
	Run:   logFn,
}

var queriesCmd = &cobra.Command{
	Use:   "queries",
	Short: "Get logged queries from ElasticSearch",
	Run:   queriesFn,
}

var clicksCmd = &cobra.Command{
	Use:   "clicks",
	Short: "Get logged clicks from ElasticSearch",
	Run:   clicksFn,
}

var latencyCmd = &cobra.Command{
	Use:   "latency",
	Short: "Get queries latency performance from ElasticSearch",
	Run:   latencyFn,
}

var elasticUrl string
var outputFile string

func init() {
	RootCmd.AddCommand(logCmd)

	logCmd.PersistentFlags().StringVar(&elasticUrl, "elastic", "", "URL of Elastic.")
	logCmd.MarkFlagRequired("elastic")
	viper.BindPFlag("elasticsearch.url", logCmd.PersistentFlags().Lookup("elastic"))

	latencyCmd.PersistentFlags().StringVar(&outputFile, "output_file", "", "CSV path to write.")

	logCmd.AddCommand(queriesCmd)
	logCmd.AddCommand(clicksCmd)
	logCmd.AddCommand(latencyCmd)
}

func logFn(cmd *cobra.Command, args []string) {
	fmt.Println("Use one of the subcommands.")
}

func initLogger() *search.SearchLogger {
	log.Infof("Setting up connection to ElasticSearch: %s", elasticUrl)
	esManager := search.MakeESManager(elasticUrl)

	return search.MakeSearchLogger(esManager)
}

func appendCsvToFile(path string, records [][]string) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0660)
	if err != nil {
		log.Fatalln("cannot open file: ", err)
	}
	defer file.Close()

	writeCsv(file, records)
}

func printCsv(records [][]string) {
	writeCsv(os.Stdout, records)
}

func writeCsv(dist io.Writer, records [][]string) {
	w := csv.NewWriter(dist)
	defer w.Flush()
	for _, record := range records {
		if err := w.Write(record); err != nil {
			log.Fatalln("error writing csv: ", err)
			return
		}
	}
}

func queriesFn(cmd *cobra.Command, args []string) {
	logger := initLogger()
	printCsv([][]string{[]string{
		"#", "SearchId", "Created", "Term", "Exact", "Filters",
		"Languages", "From", "Size", "SortBy", "Error", "Suggestion",
		"Deb"}})
	totalQueries := 0
	SLICES := 100
	for i := 0; i < SLICES; i++ {
		s := elastic.NewSliceQuery().Id(i).Max(SLICES)
		queries, err := logger.GetAllQueries(s)
		utils.Must(err)
		totalQueries += len(queries)
		sortedQueries := make(search.CreatedSearchLogs, 0, len(queries))
		for _, q := range queries {
			sortedQueries = append(sortedQueries, q)
		}
		sort.Sort(sortedQueries)
		records := [][]string{}
		for i, sl := range sortedQueries {
			filters, err := utils.PrintMap(sl.Query.Filters)
			utils.Must(err)
			records = append(records, []string{
				fmt.Sprintf("%d", i+1),
				sl.SearchId,
				sl.Created.Format("2006-01-02 15:04:05"),
				sl.Query.Term,
				strings.Join(sl.Query.ExactTerms, ","),
				filters,
				strings.Join(sl.Query.LanguageOrder, ","),
				fmt.Sprintf("%d", sl.From),
				fmt.Sprintf("%d", sl.Size),
				sl.SortBy,
				fmt.Sprintf("%t", sl.Error != nil),
				sl.Suggestion,
				fmt.Sprintf("%t", sl.Query.Deb),
			})
		}
		printCsv(records)
	}
	log.Infof("Found %d queries.", totalQueries)
}

func clicksFn(cmd *cobra.Command, args []string) {
	logger := initLogger()
	printCsv([][]string{[]string{
		"#", "SearchId", "Created", "Rank", "MdbUid", "Index", "ResultType"}})
	clicks, err := logger.GetAllClicks()
	utils.Must(err)
	log.Infof("Found %d clicks.", len(clicks))
	log.Info("#\tSearchId\tCreated\tRank\tMdbUid\tIndex\tType")
	sortedClicks := make(search.CreatedSearchClicks, 0, len(clicks))
	for _, q := range clicks {
		sortedClicks = append(sortedClicks, q)
	}
	sort.Sort(sortedClicks)
	records := [][]string{}
	for i, sq := range sortedClicks {

		records = append(records, []string{
			fmt.Sprintf("%d", i+1),
			sq.SearchId,
			sq.Created.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%d", sq.Rank),
			sq.MdbUid,
			sq.Index,
			sq.ResultType,
		})
	}
	printCsv(records)
}

func latencyFn(cmd *cobra.Command, args []string) {
	logger := initLogger()
	headers := []string{
		"#", "SearchId", "Term",
	}
	headers = append(headers, consts.LATENCY_LOG_OPERATIONS_FOR_SEARCH...)
	if outputFile != "" {
		appendCsvToFile(outputFile, [][]string{headers})
	} else {
		printCsv([][]string{headers})
	}
	totalQueries := 0
	SLICES := 100
	for i := 0; i < SLICES; i++ {
		s := elastic.NewSliceQuery().Id(i).Max(SLICES)
		queries, err := logger.GetLattestQueries(s, null.StringFrom("now-7d/d"), null.BoolFrom(false))
		utils.Must(err)
		totalQueries += len(queries)
		sortedQueries := make(search.CreatedSearchLogs, 0, len(queries))
		for _, q := range queries {
			sortedQueries = append(sortedQueries, q)
		}
		sort.Sort(sortedQueries)
		records := [][]string{}
		for i, sl := range sortedQueries {
			utils.Must(err)
			var latencies []string
			for _, op := range consts.LATENCY_LOG_OPERATIONS_FOR_SEARCH {
				hasOp := false
				for _, tl := range sl.ExecutionTimeLog {
					if tl.Operation == op {
						latancy := strconv.FormatInt(tl.Time, 10)
						latencies = append(latencies, latancy)
						hasOp = true
						break
					}
				}
				if !hasOp {
					latencies = append(latencies, "0")
				}
			}
			record := []string{
				fmt.Sprintf("%d", i+1),
				sl.SearchId,
				sl.Query.Term,
			}
			record = append(record, latencies...)
			records = append(records, record)
		}
		if outputFile != "" {
			appendCsvToFile(outputFile, records)
		} else {
			printCsv(records)
		}
	}
	log.Infof("Found %d queries.", totalQueries)
}

func latencyAggregateFn(cmd *cobra.Command, args []string) {

	const metaColumnsCnt = 3 //  "#","SearchId","Term"

	f, err := os.Open("csvPath") // TBD
	utils.Must(err)
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	utils.Must(scanner.Err())

	opLatenciesMap := make(map[string][]int, len(consts.LATENCY_LOG_OPERATIONS_FOR_SEARCH))
	for _, op := range consts.LATENCY_LOG_OPERATIONS_FOR_SEARCH {
		opLatenciesMap[op] = make([]int, len(lines)-1)
	}

	for i := 1; i < len(lines); i++ { //  skip first line (headers)
		s := strings.Split(lines[i], ",")

		for j := metaColumnsCnt; j < len(s); j++ {
			lat, err := strconv.Atoi(strings.TrimSpace(s[j]))
			utils.Must(err)
			for opIndex, op := range consts.LATENCY_LOG_OPERATIONS_FOR_SEARCH {
				if opIndex == j-metaColumnsCnt {
					opLatenciesMap[op][opIndex] = lat
				}
			}
		}
	}

	for opName, latencies := range opLatenciesMap {
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})
		sum, max := getSumAndMax(latencies)
		sum95Precent := float32(sum) * 0.95
		percentile95 := 0
		for i := 0; i < len(latencies) && float32(percentile95) < sum95Precent; i++ {
			percentile95 += latencies[i]
		}
		avg := sum / len(latencies)
		log.Infof("%s Stage\n\nAverage: %d\nWorst: %d\n95 percentile: %d.",
			opName, avg, max, percentile95)
	}

	//  TBD 5 worst queries
}

func getSumAndMax(values []int) (int, int) {
	sum := 0
	max := 0
	for _, val := range values {
		if val > max {
			max = val
		}
		sum += val
	}
	return sum, max
}
