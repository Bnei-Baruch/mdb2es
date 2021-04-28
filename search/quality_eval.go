package search

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/Bnei-Baruch/archive-backend/es"
	"io"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	SQ_BAD_STRUCTURE  = iota
	SQ_UNKNOWN        = iota
	SQ_REGULAR        = iota
	SQ_GOOD           = iota
)

var SEARCH_QUALITY_NAME = map[int]string{
	SQ_GOOD:           "Good",
	SQ_REGULAR:        "Regular",
	SQ_UNKNOWN:        "Unknown",
	SQ_BAD_STRUCTURE:  "BadStructure",
	SQ_NO_EXPECTATION: "NoExpectation",
	SQ_SERVER_ERROR:   "ServerError",
}

var SEARCH_QUALITY_BY_NAME = map[string]int{
	"Good":          SQ_GOOD,
	"Regular":       SQ_REGULAR,
	"Unknown":       SQ_UNKNOWN,
	"BadStructure":  SQ_BAD_STRUCTURE,
	"NoExpectation": SQ_NO_EXPECTATION,
	"ServerError":   SQ_SERVER_ERROR,
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
	ET_BLOG_OR_TWEET = iota
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
	ET_BLOG_OR_TWEET: true,
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
	ET_BLOG_OR_TWEET: "et_blog_or_tweet",
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
	FILTER_NAME_HOLIDAYS     = "holidays"
	PREFIX_LATEST            = "[latest]"
	BLOG_OR_TWEET_MARK       = "blog_or_tweet"
)

var FLAT_REPORT_HEADERS = []string{
	"Language", "Query", "Weight", "Bucket", "Comment",
	"Expectation", "Parsed", "SearchQuality", "Rank"}

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
	SearchQuality []int       `json:"search_quality"`
	Rank          []int       `json:"rank"`
	err           error       `json:"error"`
	QueryResult   QueryResult `json:"query_result"`
	Order         int         `json:"order,omitempty"`
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

func ReadEvalDiffSet(evalSetPath string) ([]EvalQuery, error) {
	f, err := os.Open(evalSetPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	lines, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	var ret []EvalQuery
	for _, line := range lines {
		w, err := strconv.ParseFloat(strings.TrimSpace(line[2]), 64)
		if err != nil {
			log.Warnf("Failed parsing query [%s] weight [%s].", line[1], line[2])
			continue
		}
		ret = append(ret, EvalQuery{
			Language: strings.TrimSpace(line[1]),
			Query:    strings.TrimSpace(line[0]),
			Weight:   w,
		})
	}

	return ret, nil
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
	MdbUid              string      `json:"mdb_uid"`
	ResultType          string      `json:"result_type"`
	LandingPage         string      `json:"landing_page"`
	Title               string      `json:"title,omitempty"`
	Content             string      `json:"content,omitempty"`
	CarrouselHitSources []HitSource `json:"carrousel,omitempty"`
	ContentType         string      `json:"content_type,omitempty"`
	Score               float64     `json:"score,omitempty"`
}

func HitSourcesEqual(a, b HitSource) bool {
	if a.Score == float64(0) && a.Score == b.Score {
		// All zero score are the same.
		return true
	}
	if a.MdbUid != b.MdbUid ||
		a.ResultType != b.ResultType ||
		a.LandingPage != b.LandingPage ||
		len(a.CarrouselHitSources) != len(b.CarrouselHitSources) {
		return false
	}
	for i := range a.CarrouselHitSources {
		if !HitSourcesEqual(a.CarrouselHitSources[i], b.CarrouselHitSources[i]) {
			return false
		}
	}
	return true
}

type HitDiff struct {
	Rank          int       `json:"rank"`
	ExpHitSource  HitSource `json:"exp_hit_source"`
	BaseHitSource HitSource `json:"base_hit_source"`
}

type ResultDiff struct {
	ErrorStr  string    `json:"error_str"`
	Query     string    `json:"query"`
	HitsDiffs []HitDiff `json:"hits_diffs"`
}

type ResultsDiffs struct {
	ErrorStr     string       `json:"error_str"`
	ResultsDiffs []ResultDiff `json:"results_diffs"`
	Diffs        int          `json:"diffs"`
	Scraped      int          `json:"scraped"`
	DiffsWeight  float64      `json:"diffs_weight"`
	TotalWeight  float64      `json:"total_weight"`
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
	if e == BLOG_OR_TWEET_MARK {
		return Expectation{ET_BLOG_OR_TWEET, "", nil, originalE}
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
	t := ET_NOT_SET
	if _, ok := LANDING_PAGES[landingPage]; !takeLatest && ok {
		t = ET_LANDING_PAGE
		if q == "" {
			uidOrSection = landingPage
		} else {
			subSection = landingPage
		}
	} else if _, ok := LANDING_PAGES[uidOrSection]; q == "" && !takeLatest && ok {
		t = ET_LANDING_PAGE
	} else {
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
	if e.Type == ET_BLOG_OR_TWEET {
		result := hitType == consts.ES_RESULT_TYPE_BLOG_POSTS ||
			hitType == consts.ES_RESULT_TYPE_TWEETS ||
			hitType == consts.SEARCH_RESULT_TWEETS_MANY
		return result
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

func EvaluateQuery(q EvalQuery, serverUrl string, skipExpectations bool) EvalResult {
	r := EvalResult{
		SearchQuality: make([]int, len(q.Expectations)),
		Rank:          make([]int, len(q.Expectations)),
		err:           nil,
		QueryResult:   QueryResult{},
	}

	if !skipExpectations {
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
	}

	limit := 10
	if skipExpectations {
		// We prepare this query for SxS and want to detect case where we have results with the same score
		// and thus their order changes. We want to detect this and count this as non-diff.
		limit = 20
	}
	urlTemplate := "%s/search?q=%s&language=%s&page_no=1&page_size=%d&sort_by=relevance&deb=true"
	url := fmt.Sprintf(urlTemplate, serverUrl, url.QueryEscape(q.Query), q.Language, limit)
	resp, err := http.Get(url)
	if err != nil {
		log.Warnf("Error %+v", err)
		if !skipExpectations {
			for i := range q.Expectations {
				if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
					r.SearchQuality[i] = SQ_SERVER_ERROR
				}
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
		if !skipExpectations {
			for i := range q.Expectations {
				if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
					r.SearchQuality[i] = SQ_SERVER_ERROR
				}
			}
		}
		r.err = errors.New(errMsg)
		return r
	}
	queryResult := QueryResult{}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&queryResult); err != nil {
		log.Warnf("Error decoding %+v", err)
		if !skipExpectations {
			for i := range q.Expectations {
				if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
					r.SearchQuality[i] = SQ_SERVER_ERROR
				}
			}
		}
		r.err = err
		return r
	}

	if !skipExpectations {
		for i := range q.Expectations {
			if EXPECTATIONS_FOR_EVALUATION[q.Expectations[i].Type] {
				sq := SQ_UNKNOWN
				if q.Expectations[i].Type == ET_BAD_STRUCTURE {
					sq = SQ_BAD_STRUCTURE
				}
				rank := -1
				for j, hit := range queryResult.SearchResult.Hits.Hits {
					hitSource := HitSource{}
					if hit.Type != consts.SEARCH_RESULT_TWEETS_MANY {
						if err := json.Unmarshal(*hit.Source, &hitSource); err != nil {
							log.Warnf("Error unmarshling source %+v", err)
							sq = SQ_SERVER_ERROR
							rank = -1
							r.err = err
							break
						}
					}
					if HitMatchesExpectation(hit, hitSource, q.Expectations[i]) {
						rank = j + 1
						//set good if first 3 are till 3 place in results, or 4 and 5 are till 5 place
						if j <= 2 || (i > 2 && j <= 4) {
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
	}

	if skipExpectations {
		r.QueryResult = queryResult
	}

	return r
}

func EvalScrape(queries []EvalQuery, serverUrl string, skipExpectations bool) ([]EvalResult, error) {
	log.Infof("Evaluating %d queries on %s.", len(queries), serverUrl)
	evalResults := []EvalResult(nil)

	in := make(chan EvalQuery)
	go func() {
		for i := range queries {
			in <- queries[i]
		}
		close(in)
	}()
	out := make(chan EvalResult)

	var err error
	go func() {
		err = EvalScrapeStreaming(in, out, serverUrl, skipExpectations)
	}()

	for {
		evalResult, ok := <-out
		if !ok {
			break
		} else {
			evalResults = append(evalResults, evalResult)
		}
	}

	sort.SliceStable(evalResults, func(i, j int) bool {
		return evalResults[i].Order < evalResults[j].Order
	})

	log.Infof("Finished evaluating. Returning %d results.", len(evalResults))
	return evalResults, err
}

// In case of error, this function will close the |out| channel.
// In case of |in| closed, |out| will be closed after everything scraped.
func EvalScrapeStreaming(in chan EvalQuery, out chan EvalResult, serverUrl string, skipExpectations bool) error {
	var doneWG sync.WaitGroup

	// Max inflight queries.
	paralellism := 10
	c := make(chan bool, paralellism)
	for i := 0; i < paralellism; i++ {
		c <- true
	}

	// |RATE| queries per second.
	RATE := 10
	rate := time.Second / time.Duration(RATE)
	throttle := time.Tick(rate)

	log.Infof("Scrape streaming parralellism: %d. Rate: %d per second.", len(c), RATE)

	sent := 0
	read := 0
	for {
		q, ok := <-in
		if !ok {
			log.Debugf("In is closed now (sent %d, read %d). Breaking.", sent, read)
			break
		} else {
			read++
		}

		<-throttle // Rate limit our Service.Method RPCs
		<-c        // Limit max inflight queries.
		doneWG.Add(1)

		go func(q EvalQuery, order int) {
			defer doneWG.Done()
			defer func() { c <- true }()
			evalResult := EvaluateQuery(q, serverUrl, skipExpectations)
			evalResult.Order = order
			out <- evalResult
			sent++
			log.Debugf("Done sent %d read %d (%s).", sent, read, serverUrl)
		}(q, read)
	}

	doneWG.Wait()
	log.Infof("Closing out (sent %d, read %d)", sent, read)
	close(out)
	return nil
}

func Eval(queries []EvalQuery, serverUrl string) (EvalResults, map[int][]Loss, error) {
	ret := EvalResults{}
	ret.UniqueMap = make(map[int]float64)
	ret.WeightedMap = make(map[int]float64)

	evalResults, err := EvalScrape(queries, serverUrl, false /*skipExpectations*/)
	if err != nil {
		return ret, nil, err
	}

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
	records := [][]string{FLAT_REPORT_HEADERS}
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

func WriteResultsByExpectation(path string, queries []EvalQuery, results EvalResults) ([][]string, error) {
	records := ResultsByExpectation(queries, results)
	return records, WriteToCsv(path, records)
}

func updateVsGoldenDataFromRecords(data map[string]map[string][]float64, records [][]string, isGolden bool) error {
	// Records: "Language", "Query", "Weight", "Bucket", "Comment", "Expectation", "Parsed", "SearchQuality", "Rank"
	// Assuming records are already without headers.
	for _, record := range records {
		lang := record[0]
		quality := record[7]
		qualityMap, ok := data[lang]
		if !ok {
			data[lang] = make(map[string][]float64)
			qualityMap = data[lang]
		}
		counters, ok := qualityMap[quality]
		if !ok {
			qualityMap[quality] = []float64{0.0, 0.0, 0.0, 0.0}
			counters = qualityMap[quality]
		}
		queryWeight, err := strconv.ParseFloat(record[2], 64)
		if err != nil {
			return err
		}
		if isGolden {
			counters[2]++
			counters[3] += queryWeight
		} else {
			counters[0]++
			counters[1] += queryWeight
		}
	}
	return nil
}

func diffToHtml(diff float64, round bool, percentage bool) string {
	if diff == float64(0.0) {
		return ""
	}
	percentageStr := ""
	if percentage {
		percentageStr = "%"
	}
	diffStr := fmt.Sprintf("%.2f%s", math.Abs(diff), percentageStr)
	if round {
		diffStr = fmt.Sprintf("%d%s", (int)(math.Abs(diff)), percentageStr)
	}
	if diff > 0 {
		return fmt.Sprintf("<span style='color: green'> (%s)</span>", diffStr)
	}
	return fmt.Sprintf("<span style='color: red'> (%s)</span>", diffStr)
}

func rankValue(rank string) int {
	val, err := strconv.Atoi(rank)
	if err != nil {
		val = 0
	}
	if val == -1 {
		val = 11
	}
	return val
}

func WriteVsGoldenHTML(vsGoldenHtml string, records [][]string, goldenRecords [][]string, bottomPart string) error {
	// Map from language => quality => (Unique, Weighted, Unique Golden, Weighted Golden)
	data := make(map[string]map[string][]float64)
	if err := updateVsGoldenDataFromRecords(data, records, false /*isGolden*/); err != nil {
		return err
	}
	if err := updateVsGoldenDataFromRecords(data, goldenRecords, true /*isGolden*/); err != nil {
		return err
	}

	style := `table {
		  border-collapse: collapse;
		  margin: 20px;
		}
		 th {
		  background: #ccc;
		}

		th, td {
		  border: 1px solid #ccc;
		  padding: 8px;
		}

		tr:nth-child(even) {
		  background: #efefef;
		}

		tr:hover {
		  background: #d1d1d1;
		}`
	htmlParts := []string{fmt.Sprintf("<html><style>%s</style><body><table>", style)}
	htmlParts = append(htmlParts, `
		<tr>
			<th>Language</th>
			<th>Quality</th>
			<th>Weighted%</th>
			<th>Unique%</th>
			<th>Unique</th>
		</tr>`)
	for _, language := range utils.StringMapOrderedKeys(data) {
		qualityMap := data[language]
		totalCounters := []float64{0.0, 0.0, 0.0, 0.0}
		for _, counters := range qualityMap {
			for i := 0; i < 4; i++ {
				totalCounters[i] += counters[i]
			}
		}
		firstColumn := true

		qualityKeys := []string{}
		for key := range qualityMap {
			qualityKeys = append(qualityKeys, key)
		}
		sort.SliceStable(qualityKeys, func(i, j int) bool {
			return SEARCH_QUALITY_BY_NAME[qualityKeys[i]] > SEARCH_QUALITY_BY_NAME[qualityKeys[j]]
		})

		for _, quality := range qualityKeys {
			counters := qualityMap[quality]
			if firstColumn {
				htmlParts = append(htmlParts, fmt.Sprintf(
					"<tr><td style='text-align: center; font-size: xx-large; font-weight: bold;' rowspan='%d'>%s</td>",
					len(qualityMap), language))
			} else {
				htmlParts = append(htmlParts, "<tr>")
			}
			goodStyle := ""
			if quality == "Good" {
				goodStyle = "font-size: x-large; font-weight: bold;"
			}
			tdStyle := ""
			if firstColumn {
				tdStyle = "border-top: solid black 3px;border-bottom: solid black 3px;"
				firstColumn = false
			}
			htmlParts = append(htmlParts, fmt.Sprintf(
				`   <td style='%s;%s'>%s</td>
					<td style='%s;%s'><div style="display: flex; justify-content: space-evenly"><span>%.2f%%</span>%s</div></td>
					<td style='%s'><div style="display: flex; justify-content: space-evenly"><span>%.2f%%</span>%s</div></td>
					<td style='%s'><div style="display: flex; justify-content: space-evenly"><span>%d</span>%s</div></td>
				</tr>`,
				goodStyle, tdStyle, quality,
				goodStyle, tdStyle,
				100*counters[1]/totalCounters[1], // Weighted percentage.
				diffToHtml(100*counters[1]/totalCounters[1]-100*counters[3]/totalCounters[3], false /*round*/, true /*%*/), // Weighted percentage diff.
				tdStyle,
				100*counters[0]/totalCounters[0], // Unique Percentage.
				diffToHtml(100*counters[0]/totalCounters[0]-100*counters[2]/totalCounters[2], false /*round*/, true /*%*/), // Unique percentage diff.
				tdStyle,
				(int)(counters[0]), // Unique.
				diffToHtml(counters[0]-counters[2], true /*round*/, false /*%*/), // Unique diff.
			))
		}
	}
	htmlParts = append(htmlParts, "</table>")

	// Records: "Language", "Query", "Weight", "Bucket", "Comment", "Expectation", "Parsed", "SearchQuality", "Rank"
	// Stores diffs between records and goldenRecords. Map from language to query to expectation to row.
	recordsDiff := make(map[string]map[string]map[string][][]string)
	for _, goldenRecord := range goldenRecords {
		lang := goldenRecord[0]
		query := goldenRecord[1]
		expectation := goldenRecord[5]
		if _, ok := recordsDiff[lang]; !ok {
			recordsDiff[lang] = make(map[string]map[string][][]string)
		}
		byQueryDiff := recordsDiff[lang]
		if _, ok := byQueryDiff[query]; !ok {
			byQueryDiff[query] = make(map[string][][]string)
		}
		byExpectationDiff := byQueryDiff[query]
		if _, ok := byExpectationDiff[expectation]; !ok {
			byExpectationDiff[expectation] = [][]string{}
		}
		byExpectationDiff[expectation] = append(byExpectationDiff[expectation], goldenRecord)
	}
	for _, record := range records {
		lang := record[0]
		query := record[1]
		expectation := record[5]
		if _, ok := recordsDiff[lang]; !ok {
			recordsDiff[lang] = make(map[string]map[string][][]string)
		}
		byQueryDiff := recordsDiff[lang]
		if _, ok := byQueryDiff[query]; !ok {
			byQueryDiff[query] = make(map[string][][]string)
		}
		byExpectationDiff := byQueryDiff[query]
		if _, ok := byExpectationDiff[expectation]; !ok {
			byExpectationDiff[expectation] = [][]string{}
		}
		if len(byExpectationDiff[expectation]) == 0 {
			byExpectationDiff[expectation] = append(byExpectationDiff[expectation], nil)
		}
		byExpectationDiff[expectation] = append(byExpectationDiff[expectation], record)
	}

	for _, lang := range utils.StringMapOrderedKeys(recordsDiff) {
		byQueryDiff := recordsDiff[lang]

		recordsToSort := [][][]string{}
		for _, byExpectationDiff := range byQueryDiff {
			for _, records := range byExpectationDiff {
				recordsToSort = append(recordsToSort, records)
			}
		}

		// Calculatets score for diff to order them.

		calcScore := func(p [][]string) float64 {
			if p[0] != nil && (len(p) == 1 || p[1] == nil) {
				// 100K - weight for golden only (removed).
				weight, err := strconv.ParseFloat(p[0][2], 64)
				if err != nil {
					weight = float64(0)
				}
				return float64(30000000) - weight
			}
			if p[0] == nil && p[1] != nil {
				// 200K - Weight for new only (removed).
				weight, err := strconv.ParseFloat(p[1][2], 64)
				if err != nil {
					weight = float64(0)
				}
				return float64(40000000) - weight
			}
			if p[0] != nil && p[1] != nil {
				newQuality := float64(1000000) * float64(SQ_GOOD-SEARCH_QUALITY_BY_NAME[p[1][7]]+1)
				goldenQuality := float64(1000000) * float64(SQ_GOOD-SEARCH_QUALITY_BY_NAME[p[0][7]]+1)
				weight, err := strconv.ParseFloat(p[1][2], 64)
				if err != nil {
					weight = float64(0)
				}
				if newQuality == goldenQuality {
					return 200000000 - weight
				} else if newQuality < goldenQuality {
					return newQuality - weight
				} else {
					return 100000000 + goldenQuality - weight
				}
			}
			return 0
		}

		sort.SliceStable(recordsToSort, func(i, j int) bool {
			return calcScore(recordsToSort[i]) < calcScore(recordsToSort[j])
		})

		htmlParts = append(htmlParts, "<table>")
		htmlParts = append(htmlParts, "<tr><th>Language</th><th>Query</th><th>Weight</th><th>Expectation</th><th>Quality</th><th>Rank</th></tr>")
		firstForLanguage := true
		for _, records := range recordsToSort {
			goldenRecord := records[0]
			var newRecord []string
			newRecord = nil
			if len(records) > 1 {
				newRecord = records[1]
			}
			onlyNew := goldenRecord == nil
			onlyGolden := newRecord == nil
			if newRecord == nil {
				newRecord = goldenRecord
			}

			same := true
			if !onlyNew {
				if len(goldenRecord) != len(newRecord) {
					return errors.New(fmt.Sprintf("Golden size %d is not new record size %d.", len(goldenRecord), len(newRecord)))
				}
				for i, newCell := range newRecord {
					if newCell != goldenRecord[i] {
						same = false
						break
					}
				}
			}

			if !same || onlyNew || onlyGolden {
				//log.Infof("Not same: %+v and %+v", newRecord, goldenRecord)
				htmlParts = append(htmlParts, "<tr>")
				for i, cell := range newRecord {
					style := "text-overflow: ellipsis; max-width: 200; overflow: hidden;"
					if onlyGolden {
						style = fmt.Sprintf("%s; %s", style, "color: purple;")
					} else if onlyNew {
						style = fmt.Sprintf("%s; %s", style, "color: cadetblue;")
					}
					if !onlyNew {
						goldenCell := goldenRecord[i]
						if cell != goldenCell {
							style = fmt.Sprintf("%s; %s", style, "color: blue")
							if i == 7 {
								if SEARCH_QUALITY_BY_NAME[cell] > SEARCH_QUALITY_BY_NAME[goldenCell] {
									style = fmt.Sprintf("%s; %s", style, "color: green")
									if SEARCH_QUALITY_BY_NAME[cell] == SQ_GOOD {
										style = fmt.Sprintf("%s; %s", style, "font-weight: bold")
									}
								} else if SEARCH_QUALITY_BY_NAME[cell] < SEARCH_QUALITY_BY_NAME[goldenCell] {
									style = fmt.Sprintf("%s; %s", style, "color: red")
									if SEARCH_QUALITY_BY_NAME[goldenCell] == SQ_GOOD {
										style = fmt.Sprintf("%s; %s", style, "font-weight: bold")
									}
								}
							}
							if i == 8 {
								if rankValue(cell) < rankValue(goldenCell) {
									style = fmt.Sprintf("%s; %s", style, "color: green")
								} else if rankValue(cell) > rankValue(goldenCell) {
									style = fmt.Sprintf("%s; %s", style, "color: red")
								}
							}
							cell = fmt.Sprintf("%s => %s", goldenCell, cell)
						}
					}
					rowspan := ""
					if i == 0 && firstForLanguage {
						style = fmt.Sprintf("%s; %s", style, "text-align: center; font-size: xx-large; font-weight: bold;")
						rowspan = "rowspan='0'"
					}
					if (i != 3 && i != 4 && i != 5) && (i > 0 || firstForLanguage) {
						htmlParts = append(htmlParts, fmt.Sprintf("<td %s style='%s'>%s</td>", rowspan, style, cell))
					}
				}
				htmlParts = append(htmlParts, "</tr>")
				firstForLanguage = false
			}
		}
		htmlParts = append(htmlParts, "</table>")
	}

	htmlParts = append(htmlParts, bottomPart, "</body></html>")
	html := strings.Join(htmlParts, "\n")
	return ioutil.WriteFile(vsGoldenHtml, []byte(html), 0644)
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

func evalResultToHitSources(result EvalResult) ([]HitSource, error) {
	sources := []HitSource(nil)
	if result.QueryResult.SearchResult != nil &&
		result.QueryResult.SearchResult.Hits != nil &&
		result.QueryResult.SearchResult.Hits.Hits != nil {
		for _, hit := range result.QueryResult.SearchResult.Hits.Hits {
			hitSource := HitSource{}
			if err := json.Unmarshal(*hit.Source, &hitSource); err != nil {
				// Check if hit source is carrousel (tweets for example)
				carrousel := []*elastic.SearchHit{}
				if err = json.Unmarshal(*hit.Source, &carrousel); err != nil {
					return nil, err
				}
				for i, searchHit := range carrousel {
					carrouselHitSource := HitSource{}
					if err = json.Unmarshal(*searchHit.Source, &carrouselHitSource); err != nil {
						return nil, err
					}
					if searchHit.Score != nil {
						carrouselHitSource.Score = *searchHit.Score
					}
					// Update carrousel parent required params, max score and mdb_uid.
					if i == 0 {
						hitSource.Score = carrouselHitSource.Score
						hitSource.MdbUid = carrouselHitSource.MdbUid
					}
					hitSource.CarrouselHitSources = append(hitSource.CarrouselHitSources, carrouselHitSource)
				}
			} else if hit.Type == consts.SEARCH_RESULT_LESSONS_SERIES {
				hitSource.Score = *hit.Score
				hitSource.ContentType = hit.Type
				hitSource.ResultType = hit.Type
				hitSource.MdbUid = hit.Uid

				var res es.Result
				if err := json.Unmarshal(*hit.Source, &res); err == nil {
					hitSource.Title = res.Title
				}

			} else {
				if hit.Score != nil {
					hitSource.Score = *hit.Score
				}
			}
			sources = append(sources, hitSource)
		}
		// We expect here to get list of sources with scrores. We want to break same score order by UID.
		sort.SliceStable(sources, func(i, j int) bool {
			if sources[i].Score == sources[j].Score {
				return sources[i].MdbUid < sources[j].MdbUid
			}
			return sources[i].Score > sources[j].Score
		})
		// Cut back to size of 10.
		sources = sources[:utils.Min(len(sources), 10)]
	}
	return sources, nil
}

func EvalResultDiff(evalQuery EvalQuery, expResult EvalResult, baseResult EvalResult) (ResultDiff, error) {
	ret := ResultDiff{}
	expSources, err := evalResultToHitSources(expResult)
	if err != nil {
		ret.ErrorStr = err.Error()
		return ret, err
	}
	baseSources, err := evalResultToHitSources(baseResult)
	if err != nil {
		ret.ErrorStr = err.Error()
		return ret, err
	}
	ret.Query = evalQuery.Query
	i := 0
	for i < utils.MaxInt(len(expSources), len(baseSources)) {
		if i < utils.MinInt(len(expSources), len(baseSources)) {
			if !HitSourcesEqual(expSources[i], baseSources[i]) {
				ret.HitsDiffs = append(ret.HitsDiffs, HitDiff{
					Rank:          i + 1,
					ExpHitSource:  expSources[i],
					BaseHitSource: baseSources[i],
				})
			}
		} else if i < len(expSources) {
			ret.HitsDiffs = append(ret.HitsDiffs, HitDiff{
				Rank:         i + 1,
				ExpHitSource: expSources[i],
			})
		} else { // i < len(baseSources)
			ret.HitsDiffs = append(ret.HitsDiffs, HitDiff{
				Rank:          i + 1,
				BaseHitSource: baseSources[i],
			})
		}
		i++
	}
	if len(expSources) != len(baseSources) {
		ret.ErrorStr = fmt.Sprintf("Different number of hits, exp: %d, base: %d", len(expSources), len(baseSources))
	}
	return ret, nil
}

func EvalResultDiffHtml(resultDiff ResultDiff) (string, error) {
	if len(resultDiff.HitsDiffs) == 0 && resultDiff.ErrorStr == "" {
		return "", nil
	}
	parts := []string{fmt.Sprintf("[%s]", resultDiff.Query)}
	for i := range resultDiff.HitsDiffs {
		parts = append(parts, fmt.Sprintf(
			"\t%d: %+v <> %+v",
			resultDiff.HitsDiffs[i].Rank,
			resultDiff.HitsDiffs[i].ExpHitSource,
			resultDiff.HitsDiffs[i].BaseHitSource,
		))
	}
	if resultDiff.ErrorStr != "" {
		parts = append(parts, resultDiff.ErrorStr)
	}
	return strings.Join(parts, "\n"), nil
}

func EvalResultsDiffsHtml(resultsDiffs ResultsDiffs) (string, error) {
	html := []string{}
	for i := range resultsDiffs.ResultsDiffs {
		if part, err := EvalResultDiffHtml(resultsDiffs.ResultsDiffs[i]); err != nil {
			return "", err
		} else {
			html = append(html, part)
		}
	}
	return strings.Join(html, "\n"), nil
}

func EvalResultsDiffs(evalSet []EvalQuery, expResults []EvalResult, baseResults []EvalResult) (ResultsDiffs, error) {
	ret := ResultsDiffs{}
	minLen := utils.MinInt(len(evalSet), utils.MinInt(len(expResults), len(baseResults)))
	if minLen != len(evalSet) || minLen != len(expResults) || minLen != len(baseResults) {
		ret.ErrorStr = fmt.Sprintf(
			"Expected all inputs to be of length %d, got eval set: %d, exp results: %d, base results: %d.",
			minLen, len(evalSet), len(expResults), len(baseResults))
	}
	i := 0
	for i < minLen {
		if diff, err := EvalResultDiff(evalSet[i], expResults[i], baseResults[i]); err != nil {
			ret.ErrorStr = err.Error()
			return ret, err
		} else if len(diff.HitsDiffs) != 0 || diff.ErrorStr != "" {
			ret.ResultsDiffs = append(ret.ResultsDiffs, diff)
			ret.Diffs++
			ret.DiffsWeight += evalSet[i].Weight
		}
		ret.Scraped++
		ret.TotalWeight += evalSet[i].Weight
		i++
	}
	return ret, nil
}

// Assuming |expResults| and |baseResults| are ordered by |Order| field.
func EvalResultsDiffsCount(evalSet []EvalQuery, expResults []EvalResult, baseResults []EvalResult) (int, error) {
	diffCount := 0

	expIdx := 0
	baseIdx := 0
	for expIdx < len(expResults) && baseIdx < len(baseResults) {
		baseOrder := baseResults[baseIdx].Order
		expOrder := expResults[expIdx].Order
		if baseOrder > expOrder {
			expIdx++
		} else if expOrder > baseOrder {
			baseIdx++
		} else {
			if diff, err := EvalResultDiff(evalSet[baseOrder], expResults[expIdx], baseResults[baseIdx]); err != nil {
				return 0, err
			} else if len(diff.HitsDiffs) != 0 || diff.ErrorStr != "" {
				diffCount++
			}
			expIdx++
			baseIdx++
		}
	}
	return diffCount, nil
}

func randomSelect(evalSet []EvalQuery, index int, sum float64) {
	r := rand.Float64() * sum
	swapIndex := index
	for r-evalSet[swapIndex].Weight > 0 && swapIndex < len(evalSet) {
		r -= evalSet[swapIndex].Weight
		swapIndex++
	}
	log.Debugf("Next final (%d) index: %d", index, swapIndex)
	evalSet[swapIndex], evalSet[index] = evalSet[index], evalSet[swapIndex]
}

// If |diffsLimit| > 0 will limit the diff to constant number of queries.
func EvalQuerySetDiff(evalSet []EvalQuery, baseServerUrl, expServerUrl string, diffsLimit int32) (ResultsDiffs, error) {
	if baseServerUrl == "" || expServerUrl == "" {
		errStr := "Both baseServerUrl and expServerUrl must not be empty."
		return ResultsDiffs{ErrorStr: errStr}, errors.New(errStr)
	}

	baseIn := make(chan EvalQuery)
	expIn := make(chan EvalQuery)
	diffCount := int32(0)
	evalSetQueriesUsed := 0

	totalWeight := float64(0)
	for i := range evalSet {
		totalWeight += evalSet[i].Weight
	}

	go func() {
		for ; evalSetQueriesUsed <= len(evalSet); evalSetQueriesUsed++ {
			diff := atomic.LoadInt32(&diffCount)
			if (diffsLimit > 0 && diff >= diffsLimit) || evalSetQueriesUsed == len(evalSet) {
				log.Infof("Closing input stream: (diffs) %d >= %d || (len) %d == %d ", diff, diffsLimit, evalSetQueriesUsed, len(evalSet))
				close(baseIn)
				close(expIn)
				break
			}
			randomSelect(evalSet, evalSetQueriesUsed, totalWeight)
			totalWeight -= evalSet[evalSetQueriesUsed].Weight
			// Following will sync exp and base stack to scrape in one query at a time.
			expIn <- evalSet[evalSetQueriesUsed]
			baseIn <- evalSet[evalSetQueriesUsed]
		}
	}()

	baseOut := make(chan EvalResult)
	expOut := make(chan EvalResult)

	var baseErr error
	var expErr error

	go func() {
		baseErr = EvalScrapeStreaming(baseIn, baseOut, baseServerUrl, true /*skipExpectations*/)
	}()
	go func() {
		expErr = EvalScrapeStreaming(expIn, expOut, expServerUrl, true /*skipExpectations*/)
	}()

	baseEvalResults := []EvalResult(nil)
	expEvalResults := []EvalResult(nil)

	for {
		baseEvalResult, baseOk := <-baseOut
		expEvalResult, expOk := <-expOut
		if !baseOk || !expOk {
			break
		} else {
			baseEvalResults = append(baseEvalResults, baseEvalResult)
			expEvalResults = append(expEvalResults, expEvalResult)

			// Sort results and check number of diffs.
			sort.SliceStable(baseEvalResults, func(i, j int) bool {
				return baseEvalResults[i].Order < baseEvalResults[j].Order
			})
			sort.SliceStable(expEvalResults, func(i, j int) bool {
				return expEvalResults[i].Order < expEvalResults[j].Order
			})

			// |diffCount| is shared with the goroutine adding scrapes.
			if diff, err := EvalResultsDiffsCount(evalSet, expEvalResults, baseEvalResults); err != nil {
				return ResultsDiffs{ErrorStr: err.Error()}, err
			} else {
				log.Infof("Update diff count to: %d %d %d", diff, len(expEvalResults), len(baseEvalResults))
				atomic.StoreInt32(&diffCount, int32(diff))
			}
		}
	}

	if baseErr != nil {
		return ResultsDiffs{ErrorStr: baseErr.Error()}, baseErr
	}
	if expErr != nil {
		return ResultsDiffs{ErrorStr: expErr.Error()}, expErr
	}

	// Generate eval diff.
	return EvalResultsDiffs(evalSet[:evalSetQueriesUsed], expEvalResults, baseEvalResults)
}

// Return map from filename to query set.
func ReadEvalSets(glob string) (map[string][]EvalQuery, error) {
	matches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	ret := make(map[string][]EvalQuery)
	for i := range matches {
		if evalSet, err := ReadEvalDiffSet(matches[i]); err != nil {
			return nil, err
		} else {
			ret[matches[i]] = evalSet
		}
	}
	return ret, nil
}

func EvalSearchDataQuerySetsDiff(language, baseServerUrl, expServerUrl string, diffsLimit int32) ([]ResultsDiffs, error) {
	searchDataFolder := viper.GetString("test.search-data")
	if diffsLimit <= 0 {
		diffsLimit = 200
	}
	if evalSets, err := ReadEvalSets(path.Join(searchDataFolder, fmt.Sprintf("%s*.weighted_queries.csv", language))); err != nil {
		return nil, err
	} else {
		ret := []ResultsDiffs(nil)
		for i := range evalSets {
			// TODO: Make |diffsLimit| a config variable or flag.
			if diffs, err := EvalQuerySetDiff(evalSets[i], baseServerUrl, expServerUrl, diffsLimit); err != nil {
				return nil, err
			} else {
				ret = append(ret, diffs)
			}
		}

		return ret, nil
	}
}
