package search

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/mdb"
	"github.com/Bnei-Baruch/archive-backend/utils"
	"github.com/Bnei-Baruch/sqlboiler/queries"
)

const (
	EVAL_SET_EXPECTATION_FIRST_COLUMN = 4
	EVAL_SET_EXPECTATION_LAST_COLUMN  = 8
)

// Search quality enum. Order important, the lower (higher integer) the better.
const (
	SQ_SERVER_ERROR   = iota
	SQ_NO_EXPECTATION = iota
	SQ_UNKNOWN        = iota
	SQ_REGULAR        = iota
	SQ_GOOD           = iota
)

var SEARCH_QUALITY_NAME = map[int]string{
	SQ_GOOD:           "Good",
	SQ_REGULAR:        "Regular",
	SQ_UNKNOWN:        "Unknown",
	SQ_NO_EXPECTATION: "NoExpectation",
	SQ_SERVER_ERROR:   "ServerError",
}

// Compare results classification.
const (
	CR_WIN            = iota
	CR_LOSS           = iota
	CR_SAME           = iota
	CR_NO_EXPECTATION = iota
	CR_ERROR          = iota
)

var COMPARE_RESULTS_NAME = map[int]string{
	CR_WIN:   "Win",
	CR_LOSS:  "Loss",
	CR_SAME:  "Same",
	CR_ERROR: "Error",
}

const (
	ET_NOT_SET       = -1
	ET_CONTENT_UNITS = iota
	ET_COLLECTIONS   = iota
	ET_LESSONS       = iota
	ET_PROGRAMS      = iota
	ET_SOURCES       = iota
	ET_EVENTS        = iota
	ET_LANDING_PAGE  = iota
	ET_EMPTY         = iota
	ET_FAILED_PARSE  = iota
	ET_BAD_STRUCTURE = iota
	ET_FAILED_SQL    = iota
)

var EXPECTATIONS_FOR_EVALUATION = map[int]bool{
	ET_CONTENT_UNITS: true,
	ET_COLLECTIONS:   true,
	ET_LESSONS:       true,
	ET_PROGRAMS:      true,
	ET_SOURCES:       true,
	ET_LANDING_PAGE:  true,
	ET_EMPTY:         false,
	ET_FAILED_PARSE:  false,
	ET_BAD_STRUCTURE: true,
	ET_FAILED_SQL:    true,
}

var EXPECTATION_TO_NAME = map[int]string{
	ET_CONTENT_UNITS: "et_content_units",
	ET_COLLECTIONS:   "et_collections",
	ET_LESSONS:       "et_lessons",
	ET_PROGRAMS:      "et_programs",
	ET_SOURCES:       "et_sources",
	ET_LANDING_PAGE:  "et_landing_page",
	ET_EMPTY:         "et_empty",
	ET_FAILED_PARSE:  "et_failed_parse",
	ET_BAD_STRUCTURE: "et_bad_structure",
	ET_FAILED_SQL:    "et_failed_sql",
}

var EXPECTATION_URL_PATH = map[int]string{
	ET_CONTENT_UNITS: "cu",
	ET_COLLECTIONS:   "c",
	ET_LESSONS:       "lessons",
	ET_PROGRAMS:      "programs",
	ET_SOURCES:       "sources",
	ET_EVENTS:        "events",
}

var EXPECTATION_HIT_TYPE = map[int]string{
	ET_CONTENT_UNITS: consts.ES_RESULT_TYPE_UNITS,
	ET_COLLECTIONS:   consts.ES_RESULT_TYPE_COLLECTIONS,
	ET_LESSONS:       consts.INTENT_HIT_TYPE_LESSONS,
	ET_PROGRAMS:      consts.INTENT_HIT_TYPE_PROGRAMS,
	ET_SOURCES:       consts.ES_RESULT_TYPE_SOURCES,
	ET_LANDING_PAGE:  consts.GRAMMAR_TYPE_LANDING_PAGE,
}

var LANDING_PAGES = map[string]string{
	"lessons":                   consts.GRAMMAR_INTENT_LANDING_PAGE_LESSONS,
	"lessons/daily":             consts.GRAMMAR_INTENT_LANDING_PAGE_LESSONS,
	"lessons/virtual":           consts.GRAMMAR_INTENT_LANDING_PAGE_VIRTUAL_LESSONS,
	"lessons/lectures":          consts.GRAMMAR_INTENT_LANDING_PAGE_LECTURES,
	"lessons/women":             consts.GRAMMAR_INTENT_LANDING_PAGE_WOMEN_LESSONS,
	"lessons/rabash":            consts.GRAMMAR_INTENT_LANDING_PAGE_RABASH_LESSONS,
	"lessons/series":            consts.GRAMMAR_INTENT_LANDING_PAGE_LESSON_SERIES,
	"programs/main":             consts.GRAMMAR_INTENT_LANDING_PAGE_PRORGRAMS,
	"programs/clips":            consts.GRAMMAR_INTENT_LANDING_PAGE_CLIPS,
	"sources":                   consts.GRAMMAR_INTENT_LANDING_PAGE_LIBRARY,
	"events":                    consts.GRAMMAR_INTENT_LANDING_PAGE_CONVENTIONS,
	"events/conventions":        consts.GRAMMAR_INTENT_LANDING_PAGE_CONVENTIONS,
	"events/holidays":           consts.GRAMMAR_INTENT_LANDING_PAGE_HOLIDAYS,
	"events/unity-days":         consts.GRAMMAR_INTENT_LANDING_PAGE_UNITY_DAYS,
	"events/friends-gatherings": consts.GRAMMAR_INTENT_LANDING_PAGE_FRIENDS_GATHERINGS,
	"events/meals":              consts.GRAMMAR_INTENT_LANDING_PAGE_MEALS,
	"topics":                    consts.GRAMMAR_INTENT_LANDING_PAGE_TOPICS,
	"publications/blog":         consts.GRAMMAR_INTENT_LANDING_PAGE_BLOG,
	"publications/twitter":      consts.GRAMMAR_INTENT_LANDING_PAGE_TWITTER,
	"publications/articles":     consts.GRAMMAR_INTENT_LANDING_PAGE_ARTICLES,
	"simple-mode":               consts.GRAMMAR_INTENT_LANDING_PAGE_DOWNLOADS,
	"help":                      consts.GRAMMAR_INTENT_LANDING_PAGE_HELP,
}

const (
	FILTER_NAME_SOURCE       = "source"
	FILTER_NAME_TOPIC        = "topic"
	FILTER_NAME_CONTENT_TYPE = "contentType"
	PREFIX_LATEST            = "[latest]"
)

type Filter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Expectation struct {
	Type    int      `json:"type"`
	Uid     string   `json:"uid,omitempty"`
	Filters []Filter `json:"filters,omitempty"`
	Source  string   `json:"source"`
}

type Loss struct {
	Expectation Expectation `json:"expectation,omitempty"`
	Query       EvalQuery   `json:"query,omitempty"`
	Unique      float64     `json:"unique,omitempty"`
	Weighted    float64     `json:"weighted,omitempty"`
}

type EvalQuery struct {
	Language     string        `json:"language"`
	Query        string        `json:"query"`
	Weight       float64       `json:"weight,omitempty"`
	Bucket       string        `json:"bucket,omitempty"`
	Expectations []Expectation `json:"expectations"`
	Comment      string        `json:"comment,omitempty"`
}

type EvalResults struct {
	Results       []EvalResult    `json:"results"`
	TotalUnique   uint64          `json:"total_unique"`
	TotalWeighted float64         `json:"total_weighted"`
	TotalErrors   uint64          `json:"total_errors"`
	UniqueMap     map[int]float64 `json:"unique_map"`
	WeightedMap   map[int]float64 `json:"weighted_map"`
}

type EvalResult struct {
	SearchQuality []int `json:"search_quality"`
	Rank          []int `json:"rank"`
	err           error `json:"error"`
}

// Returns compare results classification constant.
func CompareResults(base int, exp int) int {
	if base == SQ_NO_EXPECTATION || exp == SQ_NO_EXPECTATION {
		return CR_NO_EXPECTATION
	} else if base == SQ_SERVER_ERROR || exp == SQ_SERVER_ERROR {
		return CR_ERROR
	} else if base == exp {
		return CR_SAME
	} else if base < exp {
		return CR_WIN // Experiment is better
	} else {
		return CR_LOSS // Base is better
	}
}

func GoodExpectations(expectations []Expectation) int {
	ret := 0
	for i := range expectations {
		if EXPECTATIONS_FOR_EVALUATION[expectations[i].Type] {
			ret++
		}
	}
	return ret
}

func InitAndReadEvalSet(evalSetPath string) ([]EvalQuery, error) {
	db, err := sql.Open("postgres", viper.GetString("mdb.url"))
	if err != nil {
		return nil, errors.Wrap(err, "Unable to connect to DB.")
	}
	utils.Must(mdb.InitTypeRegistries(db))

	f, err := os.Open(evalSetPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return ReadEvalSet(f, db)
}

func ReadEvalSet(reader io.Reader, db *sql.DB) ([]EvalQuery, error) {
	// Read File into a Variable
	r := csv.NewReader(reader)
	lines, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	expectationsCount := 0
	queriesWithExpectationsCount := 0
	var ret []EvalQuery
	for _, line := range lines {
		w, err := strconv.ParseFloat(strings.TrimSpace(line[2]), 64)
		if err != nil {
			log.Warnf("Failed parsing query [%s] weight [%s].", line[1], line[2])
			continue
		}
		var expectations []Expectation
		hasGoodExpectations := false
		for i := EVAL_SET_EXPECTATION_FIRST_COLUMN; i <= EVAL_SET_EXPECTATION_LAST_COLUMN; i++ {
			e := ParseExpectation(strings.TrimSpace(line[i]), db)
			expectations = append(expectations, e)
			if EXPECTATIONS_FOR_EVALUATION[e.Type] {
				expectationsCount++
				hasGoodExpectations = true
			}
		}
		if hasGoodExpectations {
			queriesWithExpectationsCount++
		}
		ret = append(ret, EvalQuery{
			Language:     strings.TrimSpace(line[0]),
			Query:        strings.TrimSpace(line[1]),
			Weight:       w,
			Bucket:       strings.TrimSpace(line[3]),
			Expectations: expectations,
			Comment:      line[EVAL_SET_EXPECTATION_LAST_COLUMN+1],
		})
	}
	log.Infof("Read %d queries, with total %d expectations. %d Queries had expectations.",
		len(lines), expectationsCount, queriesWithExpectationsCount)
	return ret, nil
}

type HitSource struct {
	MdbUid      string `json:"mdb_uid"`
	ResultType  string `json:"result_type"`
	LandingPage string `json:"landing_page"`
}

// Parses expectation described by result URL and converts
// to type (collections or content_units) and uid.
// Examples:
// https://kabbalahmedia.info/he/programs/cu/AsNLozeK ==> (content_units, AsNLozeK)
// https://kabbalahmedia.info/he/programs/c/fLWpcUjQ  ==> (collections  , fLWpcUjQ)
// https://kabbalahmedia.info/he/lessons/series/c/XZoflItG  ==> (collections  , XZoflItG)
// https://kabbalahmedia.info/he/lessons?source=bs_L2jMWyce_kB3eD83I       ==> (lessons,  nil, source=bs_L2jMWyce_kB3eD83I)
// https://kabbalahmedia.info/he/programs?topic=g3ml0jum_1nyptSIo_RWqjxgkj ==> (programs, nil, topic=g3ml0jum_1nyptSIo_RWqjxgkj)
// https://kabbalahmedia.info/he/sources/kB3eD83I ==> (source, kB3eD83I)
// [latest]https://kabbalahmedia.info/he/lessons?source=bs_qMUUn22b_hFeGidcS ==> (content_units, SLQOALyt)
// [latest]https://kabbalahmedia.info/he/programs?topic=g3ml0jum_1nyptSIo_RWqjxgkj ==> (content_units, erZIsm86)
// [latest]https://kabbalahmedia.info/he/programs/c/zf4lLwyI ==> (content_units, orMKRcNk)
// All events sub pages and years:
// https://kabbalahmedia.info/he/events/meals
// https://kabbalahmedia.info/he/events/friends-gatherings
// https://kabbalahmedia.info/he/events?year=2013
func ParseExpectation(e string, db *sql.DB) Expectation {
	originalE := e
	if strings.Trim(e, " ") == "" {
		return Expectation{ET_EMPTY, "", nil, originalE}
	}
	takeLatest := strings.HasPrefix(strings.ToLower(e), PREFIX_LATEST)
	if takeLatest {
		e = e[len(PREFIX_LATEST):]
	}
	u, err := url.Parse(e)
	if err != nil {
		return Expectation{ET_FAILED_PARSE, "", nil, originalE}
	}
	p := u.RequestURI()
	idx := strings.Index(p, "?")
	q := "" // The query part, i.e., .../he/lessons?source=bs_L2jMWyce_kB3eD83I => source=bs_L2jMWyce_kB3eD83I
	if idx >= 0 {
		q = p[idx+1:]
		p = p[:idx]
	}
	// Last part .../he/programs/cu/AsNLozeK => AsNLozeK   or   /he/lessons => lessons
	uidOrSection := path.Base(p)
	// One before last part .../he/programs/cu/AsNLozeK => cu
	contentUnitOrCollection := path.Base(path.Dir(p))
	landingPage := path.Join(contentUnitOrCollection, uidOrSection)
	subSection := ""
	fmt.Printf("uidOrSection: %s, contentUnitOrCollection: %s landingPage: %s\n", uidOrSection, contentUnitOrCollection, landingPage)
	t := ET_NOT_SET
	if _, ok := LANDING_PAGES[landingPage]; q == "" && !takeLatest && ok {
		fmt.Printf("Found landing page.\n")
		t = ET_LANDING_PAGE
		uidOrSection = landingPage
	} else if _, ok := LANDING_PAGES[uidOrSection]; q == "" && !takeLatest && ok {
		t = ET_LANDING_PAGE
	} else {
		fmt.Printf("Did not find landing page.\n")
		switch uidOrSection {
		case EXPECTATION_URL_PATH[ET_LESSONS]:
			t = ET_LESSONS
		case EXPECTATION_URL_PATH[ET_PROGRAMS]:
			t = ET_PROGRAMS
		case EXPECTATION_URL_PATH[ET_EVENTS]:
			t = ET_LANDING_PAGE
			subSection = uidOrSection
		}
	}
	if t != ET_NOT_SET {
		var filters []Filter
		if q != "" {
			queryParts := strings.Split(q, "&")
			filters = make([]Filter, len(queryParts))
			for i, qp := range queryParts {
				nameValue := strings.Split(qp, "=")
				if len(nameValue) > 0 {
					filters[i].Name = nameValue[0]
					if len(nameValue) > 1 {
						filters[i].Value = nameValue[1]
					}
				}
			}
		} else {
			subSection = uidOrSection
			t = ET_LANDING_PAGE
		}
		if takeLatest {
			var err error
			var entityType string
			latestUID := ""
			if subSection == "events" {
				entityType = EXPECTATION_URL_PATH[ET_COLLECTIONS]
				latestUID, err = getLatestUIDOfCollection(consts.CT_CONGRESS, db)
			} else {
				entityType = EXPECTATION_URL_PATH[ET_CONTENT_UNITS]
				latestUID, err = getLatestUIDByFilters(filters, db)
			}
			if err != nil {
				log.Warnf("Sql Error %+v", err)
				return Expectation{ET_FAILED_SQL, "", filters, originalE}
			}
			newE := fmt.Sprintf("%s://%s%s/%s/%s", u.Scheme, u.Host, p, entityType, latestUID)
			recExpectation := ParseExpectation(newE, db)
			recExpectation.Source = originalE
			return recExpectation
		}
		return Expectation{t, subSection, filters, originalE}
	}
	if t != ET_LANDING_PAGE {
		switch contentUnitOrCollection {
		case EXPECTATION_URL_PATH[ET_CONTENT_UNITS]:
			t = ET_CONTENT_UNITS
		case EXPECTATION_URL_PATH[ET_COLLECTIONS]:
			t = ET_COLLECTIONS
			if takeLatest {
				latestUID, err := getLatestUIDByCollection(uidOrSection, db)
				if err != nil {
					log.Warnf("Sql Error %+v", err)
					return Expectation{ET_FAILED_SQL, uidOrSection, nil, originalE}
				}
				uriParts := strings.Split(p, "/")
				newE := fmt.Sprintf("%s://%s/%s/%s/%s/%s", u.Scheme, u.Host, uriParts[1], uriParts[2], EXPECTATION_URL_PATH[ET_CONTENT_UNITS], latestUID)
				recExpectation := ParseExpectation(newE, db)
				recExpectation.Source = originalE
				return recExpectation
			}
		case EXPECTATION_URL_PATH[ET_SOURCES]:
			t = ET_SOURCES
		case EXPECTATION_URL_PATH[ET_EVENTS]:
			t = ET_LANDING_PAGE
		case EXPECTATION_URL_PATH[ET_LESSONS]:
			t = ET_LANDING_PAGE
		default:
			if uidOrSection == EXPECTATION_URL_PATH[ET_SOURCES] {
				return Expectation{ET_SOURCES, "", nil, originalE}
			} else if uidOrSection == EXPECTATION_URL_PATH[ET_LESSONS] {
				return Expectation{ET_LANDING_PAGE, "", nil, originalE}
			} else {
				return Expectation{ET_BAD_STRUCTURE, "", nil, originalE}
			}
		}
	}

	if t == ET_LANDING_PAGE && takeLatest {
		var err error
		latestUID := ""
		switch uidOrSection {
		case "women":
			latestUID, err = getLatestUIDByContentType(consts.CT_WOMEN_LESSON, db)
		case "meals":
			latestUID, err = getLatestUIDByContentType(consts.CT_MEAL, db)
		case "friends-gatherings":
			latestUID, err = getLatestUIDByContentType(consts.CT_FRIENDS_GATHERING, db)
		case "lectures":
			latestUID, err = getLatestUIDByContentType(consts.CT_LECTURE, db)
		case "virtual":
			latestUID, err = getLatestUIDByContentType(consts.CT_VIRTUAL_LESSON, db)
		}
		if err != nil || latestUID == "" {
			log.Warnf("Sql Error %+v", err)
			return Expectation{ET_FAILED_SQL, uidOrSection, nil, originalE}
		}
		uriParts := strings.Split(p, "/")
		newE := fmt.Sprintf("%s://%s/%s/%s/%s/%s", u.Scheme, u.Host, uriParts[1], uriParts[2], EXPECTATION_URL_PATH[ET_CONTENT_UNITS], latestUID)
		recExpectation := ParseExpectation(newE, db)
		recExpectation.Source = originalE
		return recExpectation
	}

	if t == ET_NOT_SET {
		panic(errors.New("Expectation not set."))
	}
	return Expectation{t, uidOrSection, nil, originalE}
}

func FilterValueToUid(value string) string {
	sl := strings.Split(value, "_")
	if len(sl) == 0 {
		return ""
	}
	return sl[len(sl)-1]
}

func HitMatchesExpectation(hit *elastic.SearchHit, hitSource HitSource, e Expectation) bool {
	hitType := hit.Type
	if hitType == "result" {
		hitType = hitSource.ResultType
	}
	if hitType != EXPECTATION_HIT_TYPE[e.Type] {
		return false
	}

	if e.Type == ET_LESSONS || e.Type == ET_PROGRAMS {
		// For now we support only one filter (zero also means not match).
		if len(e.Filters) == 0 || len(e.Filters) > 1 {
			return false
		}
		// Match all filters
		filter := e.Filters[0]
		return ((filter.Name == FILTER_NAME_TOPIC && hit.Index == consts.INTENT_INDEX_TAG) ||
			(filter.Name == FILTER_NAME_SOURCE && hit.Index == consts.INTENT_INDEX_SOURCE)) &&
			FilterValueToUid(filter.Value) == hitSource.MdbUid
	} else if e.Type == ET_LANDING_PAGE {
		return LANDING_PAGES[e.Uid] == hitSource.LandingPage
	} else {
		return hitSource.MdbUid == e.Uid
	}
}

func EvaluateQuery(q EvalQuery, serverUrl string) EvalResult {
	r := EvalResult{
		SearchQuality: make([]int, len(q.Expectations)),
		Rank:          make([]int, len(q.Expectations)),
		err:           nil,
	}

	hasGoodExpectation := false
	for i := range q.Expectations {
		r.SearchQuality[i] = SQ_NO_EXPECTATION
		r.Rank[i] = -1
		if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
			hasGoodExpectation = true
		}
	}
	// Optimization, don't fetch query if no good expectations.
	if !hasGoodExpectation {
		return r
	}

	urlTemplate := "%s/search?q=%s&language=%s&page_no=1&page_size=10&sort_by=relevance&deb=true"
	url := fmt.Sprintf(urlTemplate, serverUrl, url.QueryEscape(q.Query), q.Language)
	resp, err := http.Get(url)
	if err != nil {
		log.Warnf("Error %+v", err)
		for i := range q.Expectations {
			if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
				r.SearchQuality[i] = SQ_SERVER_ERROR
			}
		}
		r.err = err
		return r
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			r.err = errors.Wrapf(err, "Status not ok (%d), failed reading body. Url: %s.", resp.StatusCode, url)
		}
		errMsg := fmt.Sprintf("Status not ok (%d), body: %s, url: %s.", resp.StatusCode, string(bodyBytes), url)
		log.Warn(errMsg)
		for i := range q.Expectations {
			if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
				r.SearchQuality[i] = SQ_SERVER_ERROR
			}
		}
		r.err = errors.New(errMsg)
		return r
	}
	queryResult := QueryResult{}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&queryResult); err != nil {
		log.Warnf("Error decoding %+v", err)
		for i := range q.Expectations {
			if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
				r.SearchQuality[i] = SQ_SERVER_ERROR
			}
		}
		r.err = err
		return r
	}
	for i := range q.Expectations {
		if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
			sq := SQ_UNKNOWN
			rank := -1
			for j, hit := range queryResult.SearchResult.Hits.Hits {
				hitSource := HitSource{}
				if err := json.Unmarshal(*hit.Source, &hitSource); err != nil {
					log.Warnf("Error unmarshling source %+v", err)
					sq = SQ_SERVER_ERROR
					rank = -1
					r.err = err
					break
				}
				if HitMatchesExpectation(hit, hitSource, q.Expectations[i]) {
					rank = j + 1
					if j <= 2 {
						sq = SQ_GOOD
					} else {
						sq = SQ_REGULAR
					}
					break
				}
			}
			r.SearchQuality[i] = sq
			r.Rank[i] = rank
		}
	}

	return r
}

func Eval(queries []EvalQuery, serverUrl string) (EvalResults, map[int][]Loss, error) {
	log.Infof("Evaluating %d queries on %s.", len(queries), serverUrl)
	ret := EvalResults{}
	ret.UniqueMap = make(map[int]float64)
	ret.WeightedMap = make(map[int]float64)

	evalResults := make([]EvalResult, len(queries))

	var doneWG sync.WaitGroup
	paralellism := 5
	c := make(chan bool, paralellism)
	for i := 0; i < paralellism; i++ {
		c <- true
	}
	log.Infof("C: %d", len(c))
	rate := time.Second / 10
	throttle := time.Tick(rate)
	for i, q := range queries {
		<-throttle // rate limit our Service.Method RPCs
		<-c
		doneWG.Add(1)
		go func(i int, q EvalQuery) {
			defer doneWG.Done()
			defer func() { c <- true }()
			evalResults[i] = EvaluateQuery(q, serverUrl)
			log.Infof("Done %d / %d", i, len(queries))
		}(i, q)
	}
	doneWG.Wait()

	for i, r := range evalResults {
		q := queries[i]
		goodExpectations := GoodExpectations(q.Expectations)
		if goodExpectations > 0 {
			for i, sq := range r.SearchQuality {
				if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
					ret.UniqueMap[sq] += 1 / float64(goodExpectations)
					// Each expectation has equal weight for the query.
					ret.WeightedMap[sq] += float64(q.Weight) / float64(goodExpectations)
				}
			}
		} else {
			// Meaning that the query has not any good expectation.
			ret.UniqueMap[SQ_NO_EXPECTATION]++
			ret.WeightedMap[SQ_NO_EXPECTATION] += float64(q.Weight)
		}
		ret.TotalUnique++
		ret.TotalWeighted += q.Weight
		if r.err != nil {
			ret.TotalErrors++
		}
		ret.Results = append(ret.Results, r)
		if len(ret.Results)%20 == 0 {
			log.Infof("Done evaluating (%d/%d) queries.", len(ret.Results), len(queries))
		}
	}
	for k, v := range ret.UniqueMap {
		ret.UniqueMap[k] = v / float64(ret.TotalUnique)
	}
	for k, v := range ret.WeightedMap {
		ret.WeightedMap[k] = v / float64(ret.TotalWeighted)
	}

	// Print detailed loss (Unknown) analysis
	losses := make(map[int][]Loss)
	for i, q := range queries {
		for j, sq := range ret.Results[i].SearchQuality {
			e := q.Expectations[j]
			goodExpectationsLen := GoodExpectations(q.Expectations)
			if sq == SQ_UNKNOWN || sq == SQ_SERVER_ERROR {
				if _, ok := losses[e.Type]; !ok {
					losses[e.Type] = make([]Loss, 0)
				}
				loss := Loss{e, q, 1 / float64(goodExpectationsLen), float64(q.Weight) / float64(goodExpectationsLen)}
				losses[e.Type] = append(losses[e.Type], loss)
			}
		}
	}

	return ret, losses, nil
}

func ExpectationToString(e Expectation) string {
	filters := make([]string, len(e.Filters))
	for i, f := range e.Filters {
		filters[i] = fmt.Sprintf("%s - %s", f.Name, f.Value)
	}
	return fmt.Sprintf("%s|%s|%s", EXPECTATION_TO_NAME[e.Type], e.Uid, strings.Join(filters, ":"))
}

func ResultsByExpectation(queries []EvalQuery, results EvalResults) [][]string {
	records := [][]string{{"Language", "Query", "Weight", "Bucket", "Comment",
		"Expectation", "Parsed", "SearchQuality", "Rank"}}
	for i, q := range queries {
		goodExpectationsLen := GoodExpectations(q.Expectations)
		for j, sq := range results.Results[i].SearchQuality {
			if EXPECTATIONS_FOR_EVALUATION[q.Expectations[j].Type] {
				record := []string{q.Language, q.Query, fmt.Sprintf("%.2f", float64(q.Weight)/float64(goodExpectationsLen)),
					q.Bucket, q.Comment, q.Expectations[j].Source, ExpectationToString(q.Expectations[j]),
					SEARCH_QUALITY_NAME[sq], fmt.Sprintf("%d", results.Results[i].Rank[j])}
				records = append(records, record)
			}
		}
	}
	return records
}

func WriteResultsByExpectation(path string, queries []EvalQuery, results EvalResults) error {
	return WriteToCsv(path, ResultsByExpectation(queries, results))
}

func WriteResults(path string, queries []EvalQuery, results EvalResults) error {
	records := [][]string{{"Language", "Query", "Weight", "Bucket", "Comment"}}
	for i := 0; i < EVAL_SET_EXPECTATION_LAST_COLUMN-EVAL_SET_EXPECTATION_FIRST_COLUMN+1; i++ {
		records[0] = append(records[0], fmt.Sprintf("#%d", i+1))
		records[0] = append(records[0], fmt.Sprintf("#%d Parsed", i+1))
		records[0] = append(records[0], fmt.Sprintf("#%d SQ", i+1))
		records[0] = append(records[0], fmt.Sprintf("#%d Rank", i+1))
	}
	for i, q := range queries {
		record := []string{q.Language, q.Query, fmt.Sprintf("%.2f", q.Weight), q.Bucket, q.Comment}
		for j, sq := range results.Results[i].SearchQuality {
			record = append(record, q.Expectations[j].Source)
			record = append(record, ExpectationToString(q.Expectations[j]))
			record = append(record, SEARCH_QUALITY_NAME[sq])
			record = append(record, fmt.Sprintf("%d", results.Results[i].Rank[j]))
		}
		records = append(records, record)
	}

	return WriteToCsv(path, records)
}

func CsvToString(records [][]string) (error, string) {
	buf := new(bytes.Buffer)
	w := csv.NewWriter(buf)

	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err, ""
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return err, ""
	}

	return nil, buf.String()
}

func WriteToCsv(path string, records [][]string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w := csv.NewWriter(file)

	for _, record := range records {
		if err := w.Write(record); err != nil {
			return err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return nil
}

func getLatestUIDByCollection(collectionUID string, db *sql.DB) (string, error) {
	var latestUID string

	queryMask := `select cu.uid from content_units cu
		join collections_content_units ccu on cu.id = ccu.content_unit_id
		join collections c on c.id = ccu.collection_id
		where cu.published IS TRUE and cu.secure = 0
			and cu.type_id NOT IN (%d, %d, %d, %d, %d, %d, %d)
		and c.uid = '%s'
		order by ccu.position desc
			limit 1`

	query := fmt.Sprintf(queryMask,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_CLIP].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LELO_MIKUD].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_PUBLICATION].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SONG].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BOOK].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BLOG_POST].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_UNKNOWN].ID,
		collectionUID)

	row := queries.Raw(db, query).QueryRow()

	err := row.Scan(&latestUID)
	if err != nil {
		return "", errors.Wrap(err, "Unable to retrieve from DB the latest content unit UID by collection.")
	}

	return latestUID, nil
}

func getLatestUIDByFilters(filters []Filter, db *sql.DB) (string, error) {
	queryMask := `
		select cu.uid from content_units cu
		left join content_units_tags cut on cut.content_unit_id = cu.id
		left join tags t on t.id = cut.tag_id
		left join content_units_sources cus on cus.content_unit_id = cu.id
		left join sources s on s.id = cus.source_id
		where cu.published IS TRUE and cu.secure = 0
		and cu.type_id NOT IN (%d, %d, %d, %d, %d, %d, %d)
		%s
		order by (cu.properties->>'film_date')::date desc
		limit 1`

	var uid string
	filterByUidQuery := ""
	sourceUids := make([]string, 0)
	tagsUids := make([]string, 0)
	contentType := ""
	query := ""

	if len(filters) > 0 {
		for _, filter := range filters {
			switch filter.Name {
			case FILTER_NAME_SOURCE:
				uidStr := fmt.Sprintf("'%s'", FilterValueToUid(filter.Value))
				sourceUids = append(sourceUids, uidStr)
			case FILTER_NAME_TOPIC:
				uidStr := fmt.Sprintf("'%s'", FilterValueToUid(filter.Value))
				tagsUids = append(tagsUids, uidStr)
			case FILTER_NAME_CONTENT_TYPE:
				contentType = filter.Value
			}
		}
	} else {
		contentType = consts.CT_LESSON_PART
	}

	if len(sourceUids) > 0 {
		filterByUidQuery += fmt.Sprintf(`and s.id in (select AA.id from (
            WITH RECURSIVE rec_sources AS (
                SELECT id, parent_id FROM sources s
                    WHERE uid in (%s)
                UNION SELECT
                    s.id, s.parent_id
                FROM sources s INNER JOIN rec_sources rs ON s.parent_id = rs.id
            )
            SELECT id FROM rec_sources) AS AA) `, strings.Join(sourceUids, ","))
	}
	if len(tagsUids) > 0 {
		filterByUidQuery += fmt.Sprintf(`and t.id in (select AA.id from (
            WITH RECURSIVE rec_tags AS (
                SELECT id, parent_id FROM tags t
                    WHERE uid in (%s)
                UNION SELECT
                    t.id, t.parent_id
                FROM tags t INNER JOIN rec_tags rt ON t.parent_id = rt.id
            )
            SELECT id FROM rec_tags) AS AA) `, strings.Join(tagsUids, ","))
	}
	if contentType != "" {
		filterByUidQuery += fmt.Sprintf("and cu.type_id = %d ", mdb.CONTENT_TYPE_REGISTRY.ByName[contentType].ID)
	}

	query += fmt.Sprintf(queryMask,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_CLIP].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_LELO_MIKUD].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_PUBLICATION].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_SONG].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BOOK].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_BLOG_POST].ID,
		mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_UNKNOWN].ID,
		filterByUidQuery)

	row := queries.Raw(db, query).QueryRow()

	err := row.Scan(&uid)
	if err != nil {
		return "", errors.Wrap(err, "Unable to retrieve from DB the latest UID for lesson by tag or by source or by content type.")
	}

	return uid, nil

}

func getLatestUIDByContentType(cT string, db *sql.DB) (string, error) {
	return getLatestUIDByFilters([]Filter{Filter{Name: FILTER_NAME_CONTENT_TYPE, Value: cT}}, db)
}

func getLatestUIDOfCollection(contentType string, db *sql.DB) (string, error) {

	var uid string

	queryMask :=
		`select c.uid from collections c
		where c.published IS TRUE and c.secure = 0
		and c.type_id = %d
		order by (c.properties->>'film_date')::date desc
		limit 1`

	contentTypeId := mdb.CONTENT_TYPE_REGISTRY.ByName[contentType].ID
	query := fmt.Sprintf(queryMask, contentTypeId)

	row := queries.Raw(db, query).QueryRow()

	err := row.Scan(&uid)
	if err != nil {
		return "", errors.Wrap(err, "Unable to retrieve from DB the latest UID for collection by content type.")
	}

	return uid, nil

}
