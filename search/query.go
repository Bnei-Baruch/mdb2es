package search

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/Bnei-Baruch/archive-backend/utils"
	"github.com/pkg/errors"
)

const (
	// Content boost.
	TITLE_BOOST       = 2.0
	DESCRIPTION_BOOST = 1.2
	FULL_TITLE_BOOST  = 1.1
	DEFAULT_BOOST     = 1.0

	// Max slop.
	SLOP = 100

	// Following two boosts may be agregated.
	// Boost for standard anylyzer, i.e., without stemming.
	STANDARD_BOOST = 1.2
	// Boost for exact phrase match, without slop.
	EXACT_BOOST = 1.5

	SPAN_NEAR_BOOST = 0.01

	MIN_SCORE_FOR_RESULTS = 0.01

	NUM_SUGGESTS = 30
)

type Query struct {
	Term          string              `json:"term,omitempty"`
	ExactTerms    []string            `json:"exact_terms,omitempty"`
	Original      string              `json:"original,omitempty"`
	Filters       map[string][]string `json:"filters,omitempty"`
	LanguageOrder []string            `json:"language_order,omitempty"`
	Deb           bool                `json:"deb,omitempty"`
	Intents       []Intent            `json:"intents,omitempty"`
}

func isTokenStart(i int, runes []rune, lastQuote rune) bool {
	return i == 0 && !unicode.IsSpace(runes[0]) ||
		(i > 0 && !unicode.IsSpace(runes[i]) && unicode.IsSpace(runes[i-1]))
}

func isTokenEnd(i int, runes []rune, lastQuote rune, lastQuoteIdx int) bool {
	return i == len(runes)-1 ||
		(i < len(runes)-1 && unicode.IsSpace(runes[i+1]) &&
			(lastQuote == rune(0) || runes[i] == lastQuote && lastQuoteIdx >= 0 && lastQuoteIdx < i))
}

func isRuneQuotationMark(r rune) bool {
	return unicode.In(r, unicode.Quotation_Mark) || r == rune(1523) || r == rune(1524)
}

var SINGLE_QUOTE_RUNES = map[rune]bool{
	rune(39):   true,
	rune(145):  true,
	rune(146):  true,
	rune(8216): true,
	rune(8217): true,
	rune(8219): true,
	rune(1523): true,
	rune(1436): true,
	rune(1437): true,
}

func doubleSingleQuotesToDoubleQuotes(runes []rune) []rune {
	ret := []rune(nil)
	i := 0
	for ; i < len(runes)-1; i++ {
		_, quoteFirst := SINGLE_QUOTE_RUNES[runes[i]]
		_, quoteSecond := SINGLE_QUOTE_RUNES[runes[i+1]]
		if quoteFirst && quoteSecond {
			ret = append(ret, rune(34))
			i++
		} else {
			ret = append(ret, runes[i])
		}
	}
	if i < len(runes) {
		ret = append(ret, runes[i])
	}
	return ret
}

// Tokenizes string to work with user friendly escapings of quotes (see tests).
func tokenize(str string) []string {
	runes := doubleSingleQuotesToDoubleQuotes([]rune(str))
	start := -1
	lastQuote := rune(0)
	lastQuoteIdx := -1
	parts := 0
	var tokens []string
	for i, r := range runes {
		if start == -1 && isTokenStart(i, runes, lastQuote) {
			start = i
		}
		if i == start && lastQuote == rune(0) && isRuneQuotationMark(r) {
			for k := i + 1; k < len(runes); k++ { // Make sure we have closing QuotationMark
				if isTokenEnd(k, runes, r, i) && isRuneQuotationMark(runes[k]) {
					// Closing QuotationMark found
					lastQuote = r
					lastQuoteIdx = i
					break
				}
			}
		}
		if start >= 0 && isTokenEnd(i, runes, lastQuote, lastQuoteIdx) {
			tokens = append(tokens, string(runes[start:i+1]))
			lastQuote = rune(0)
			lastQuoteIdx = -1
			start = -1
			parts += 1
		}
	}

	return tokens
}

// Parses query and extracts terms and filters.
func ParseQuery(q string) Query {
	filters := make(map[string][]string)
	var terms []string
	var exactTerms []string
	for _, t := range tokenize(q) {
		isFilter := false
		for filter := range consts.FILTERS {
			prefix := fmt.Sprintf("%s:", filter)
			if isFilter = strings.HasPrefix(t, prefix); isFilter {
				filters[consts.FILTERS[filter]] = strings.Split(strings.TrimPrefix(t, prefix), ",")
				break
			}
		}
		if !isFilter {
			// Not clear what kind of decoding is happening here, utf-8?!
			runes := []rune(t)
			// For debug
			// for _, c := range runes {
			//     fmt.Printf("%04x %s\n", c, string(c))
			// }
			if len(runes) >= 2 && isRuneQuotationMark(runes[0]) && runes[0] == runes[len(runes)-1] {
				exactTerms = append(exactTerms, string(runes[1:len(runes)-1]))
			} else {
				terms = append(terms, t)
			}
		}
	}
	return Query{Term: strings.Join(terms, " "), ExactTerms: exactTerms, Original: q, Filters: filters}
}

// Here we build the span_near query with span_multi sub queries to allow effective fuzzy search
//  with slop and special cases like avoiding numeric values from applying fuzziness and handling of single hebrew letter in the query.
// The query is not supported in the elastic SDK so we build it manually.
// Arguments:
// field - the field where we search (title, full_title, description, content).
// term - the search term.
// boost - boost for the score.
// slop - number of words separating the span clauses.
// inOrder - we expect the words to be in order? We set it as true only for the search in title.
func createSpanNearQuery(field string, term string, boost float32, slop int, inOrder bool) (elastic.Query, error) {
	clauses := make([]string, 0)
	spanNearMask := `{"span_near": { "clauses": [%s], "slop": %d, "boost": %f, "in_order": %t } }`
	clauseMask := `{"span_multi": { "match": { "fuzzy": { "%s": { "value": "%s", "fuzziness": %s, "transpositions": %s } } } } }`
	splitted := strings.Fields(term)
	for _, t := range splitted {
		if t == "<" || t == ">" || t == "-" {
			continue
		}
		fuzzines := `"AUTO"`     // Default.
		transpositions := "true" // Default.
		runes := []rune(t)
		_, convertToIntErr := strconv.Atoi(t)
		if convertToIntErr == nil || (len(runes) == 3 && runes[1] == '"') || (len(runes) == 4 && runes[2] == '"') {
			//  We dont use fuzzines for numeric values (number or hebrew numeric representation like מ"ג)
			fuzzines = "0"
		} else if len(runes) == 1 && runes[0] >= 'א' && runes[0] <= 'ת' {
			// This logic allows finding single hebrew letter with ' symbol without the mention of the ' symbol.
			// The solution is not perfect for all times. In some (rare) cases the letter may be replaced with another letter: ג' קווים - ד
			fuzzines = "1"
			transpositions = "false" // Limit the fuzzines not to include transpositions of two adjacent characters (ח' -> 'ח). Maybe not required.
		}
		b, err := json.Marshal(t)
		if err != nil {
			return nil, errors.Wrap(err, "createSpanNearQuery")
		}
		// Trim the beginning and trailing " character
		esc := string(b[1 : len(b)-1])
		clause := fmt.Sprintf(clauseMask, field, esc, fuzzines, transpositions)
		clauses = append(clauses, clause)
	}
	clausesStr := strings.Join(clauses, ",")
	queryStr := fmt.Sprintf(spanNearMask, clausesStr, slop, boost, inOrder)
	//fmt.Printf("SpanNear Query: %s\n", queryStr)
	query := elastic.NewRawStringQuery(queryStr)
	return query, nil
}

func addMustNotSeries(q Query) *elastic.BoolQuery {
	if filters, ok := q.Filters[consts.FILTERS[consts.FILTER_COLLECTIONS_CONTENT_TYPES]]; ok {
		for _, f := range filters {
			if f == consts.CT_LESSONS_SERIES {
				return nil
			}
		}
	}
	//remove from results lesson series collections
	fCollections := elastic.NewTermsQuery(consts.ES_RESULT_TYPE, consts.ES_RESULT_TYPE_COLLECTIONS)
	fSeries := elastic.NewTermsQuery("filter_values", fmt.Sprintf("%s:%s", consts.FILTERS[consts.FILTER_COLLECTIONS_CONTENT_TYPES], consts.CT_LESSONS_SERIES))
	return elastic.NewBoolQuery().Filter(fCollections, fSeries)
}

// Creates a result query for elastic.
// resultTypes - list of search result types: sources, topics, CU's, etc..
// docIds - optional list of _uid's for filtering the search. If the parameter value is nil, no filtering is applied. Used for highlight search.
// filterOutCUSources - optional list of sources for which we want to filter out the CU's that connected to those sources
//	(in order to avoid duplication between carousel and regular results).
// titlesOnly - limit our search only to title fields: title, full_title and description in case we search for intent sources. Used for intent search.
func createResultsQuery(resultTypes []string, q Query, docIds []string, filterOutCUSources []string, titlesOnly bool) (elastic.Query, error) {
	boolQuery := elastic.NewBoolQuery().Must(
		elastic.NewConstantScoreQuery(
			elastic.NewTermsQuery("result_type", utils.ConvertArgsString(resultTypes)...),
		).Boost(0.0),
	)
	if docIds != nil && len(docIds) > 0 {
		idsQuery := elastic.NewIdsQuery().Ids(docIds...)
		boolQuery.Filter(idsQuery)
	}
	if len(filterOutCUSources) > 0 {
		rtForMustNotQuery := elastic.NewTermsQuery(consts.ES_RESULT_TYPE, consts.ES_RESULT_TYPE_UNITS)
		for _, src := range filterOutCUSources {
			sourceForMustNotQuery := elastic.NewTermsQuery("typed_uids", fmt.Sprintf("%s:%s", consts.FILTER_SOURCE, src))
			innerBoolQuery := elastic.NewBoolQuery().Filter(sourceForMustNotQuery, rtForMustNotQuery)
			boolQuery.MustNot(innerBoolQuery)
		}
	}

	if mustNot := addMustNotSeries(q); mustNot != nil {
		boolQuery.MustNot(mustNot)
	}

	//  We append description for intent sources search because the description is commonly used as subtitle
	appendDecription := !titlesOnly || (len(resultTypes) == 1 && resultTypes[0] == consts.ES_RESULT_TYPE_SOURCES)
	if q.Term != "" {

		constantScoreQueries := []elastic.Query{
			elastic.NewMatchQuery("title.language", q.Term),
			elastic.NewMatchQuery("full_title.language", q.Term),
		}
		if appendDecription {
			constantScoreQueries = append(constantScoreQueries,
				elastic.NewMatchQuery("description.language", q.Term),
			)
		}
		if !titlesOnly {
			constantScoreQueries = append(constantScoreQueries,
				elastic.NewMatchQuery("content.language", q.Term),
			)
		}

		disMaxQueries := []elastic.Query{
			// Language analyzed
			elastic.NewMatchPhraseQuery("title.language", q.Term).Slop(SLOP).Boost(TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title.language", q.Term).Slop(SLOP).Boost(FULL_TITLE_BOOST),

			// Language analyzed, exact (no slop)
			elastic.NewMatchPhraseQuery("title.language", q.Term).Boost(EXACT_BOOST * TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title.language", q.Term).Boost(EXACT_BOOST * FULL_TITLE_BOOST),

			// Standard analyzed
			elastic.NewMatchPhraseQuery("title", q.Term).Slop(SLOP).Boost(STANDARD_BOOST * TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title", q.Term).Slop(SLOP).Boost(STANDARD_BOOST * FULL_TITLE_BOOST),

			// Standard analyzed, exact (no slop).
			elastic.NewMatchPhraseQuery("title", q.Term).Boost(STANDARD_BOOST * EXACT_BOOST * TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title", q.Term).Boost(STANDARD_BOOST * EXACT_BOOST * FULL_TITLE_BOOST),
		}

		// Language analyzed
		snq, err := createSpanNearQuery("title.language", q.Term, TITLE_BOOST*SPAN_NEAR_BOOST, SLOP, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)
		snq, err = createSpanNearQuery("full_title.language", q.Term, FULL_TITLE_BOOST*SPAN_NEAR_BOOST, SLOP, false)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)

		// Language analyzed, exact (no slop)
		snq, err = createSpanNearQuery("title.language", q.Term, EXACT_BOOST*TITLE_BOOST*SPAN_NEAR_BOOST, 0, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)
		snq, err = createSpanNearQuery("full_title.language", q.Term, EXACT_BOOST*FULL_TITLE_BOOST*SPAN_NEAR_BOOST, 0, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)

		// Standard analyzed
		snq, err = createSpanNearQuery("title", q.Term, STANDARD_BOOST*TITLE_BOOST*SPAN_NEAR_BOOST, SLOP, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)
		snq, err = createSpanNearQuery("full_title", q.Term, STANDARD_BOOST*FULL_TITLE_BOOST*SPAN_NEAR_BOOST, SLOP, false)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)

		// Standard analyzed, exact (no slop).
		snq, err = createSpanNearQuery("title", q.Term, STANDARD_BOOST*EXACT_BOOST*TITLE_BOOST*SPAN_NEAR_BOOST, 0, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)
		snq, err = createSpanNearQuery("full_title", q.Term, STANDARD_BOOST*EXACT_BOOST*FULL_TITLE_BOOST*SPAN_NEAR_BOOST, 0, true)
		if err != nil {
			return nil, err
		}
		disMaxQueries = append(disMaxQueries, snq)

		if appendDecription {
			disMaxQueries = append(disMaxQueries,
				// Language analyzed
				elastic.NewMatchPhraseQuery("description.language", q.Term).Slop(SLOP).Boost(DESCRIPTION_BOOST),

				// Language analyzed, exact (no slop)
				elastic.NewMatchPhraseQuery("description.language", q.Term).Boost(EXACT_BOOST*DESCRIPTION_BOOST),

				// Standard analyzed
				elastic.NewMatchPhraseQuery("description", q.Term).Slop(SLOP).Boost(STANDARD_BOOST*DESCRIPTION_BOOST),

				// Standard analyzed, exact (no slop).
				elastic.NewMatchPhraseQuery("description", q.Term).Boost(STANDARD_BOOST*EXACT_BOOST*DESCRIPTION_BOOST),
			)
			// Language analyzed
			snq, err = createSpanNearQuery("description.language", q.Term, DESCRIPTION_BOOST*SPAN_NEAR_BOOST, SLOP, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Language analyzed, exact (no slop)
			snq, err = createSpanNearQuery("description.language", q.Term, EXACT_BOOST*DESCRIPTION_BOOST*SPAN_NEAR_BOOST, 0, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Standard analyzed
			snq, err = createSpanNearQuery("description", q.Term, STANDARD_BOOST*DESCRIPTION_BOOST*SPAN_NEAR_BOOST, SLOP, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Standard analyzed, exact (no slop).
			snq, err = createSpanNearQuery("description", q.Term, STANDARD_BOOST*EXACT_BOOST*DESCRIPTION_BOOST*SPAN_NEAR_BOOST, 0, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)
		}
		if !titlesOnly {
			disMaxQueries = append(disMaxQueries,
				// Language analyzed
				elastic.NewMatchPhraseQuery("content.language", q.Term).Slop(SLOP),

				// Language analyzed, exact (no slop)
				elastic.NewMatchPhraseQuery("content.language", q.Term).Boost(EXACT_BOOST),

				// Standard analyzed
				elastic.NewMatchPhraseQuery("content", q.Term).Slop(SLOP).Boost(STANDARD_BOOST),

				// Standard analyzed, exact (no slop).
				elastic.NewMatchPhraseQuery("content", q.Term).Boost(STANDARD_BOOST*EXACT_BOOST),
			)

			// Language analyzed
			snq, err = createSpanNearQuery("content.language", q.Term, DEFAULT_BOOST*SPAN_NEAR_BOOST, SLOP, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Language analyzed, exact (no slop)
			snq, err = createSpanNearQuery("content.language", q.Term, EXACT_BOOST*SPAN_NEAR_BOOST, 0, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Standard analyzed
			snq, err = createSpanNearQuery("content", q.Term, STANDARD_BOOST*SPAN_NEAR_BOOST, SLOP, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)

			// Standard analyzed, exact (no slop).
			snq, err = createSpanNearQuery("content", q.Term, STANDARD_BOOST*EXACT_BOOST*SPAN_NEAR_BOOST, 0, true)
			if err != nil {
				return nil, err
			}
			disMaxQueries = append(disMaxQueries, snq)
		}

		boolQuery = boolQuery.Must(
			// Don't calculate score here, as we use sloped score below.
			elastic.NewConstantScoreQuery(
				elastic.NewBoolQuery().Should(constantScoreQueries...).MinimumNumberShouldMatch(1),
			).Boost(0.0),
		).Should(
			elastic.NewDisMaxQuery().Query(disMaxQueries...),
		)
	}
	for _, exactTerm := range q.ExactTerms {

		constantScoreQueries := []elastic.Query{
			elastic.NewMatchPhraseQuery("title", exactTerm),
			elastic.NewMatchPhraseQuery("full_title", exactTerm),
		}
		if appendDecription {
			constantScoreQueries = append(constantScoreQueries,
				elastic.NewMatchPhraseQuery("description", exactTerm),
			)
		}
		if !titlesOnly {
			constantScoreQueries = append(constantScoreQueries,
				elastic.NewMatchPhraseQuery("content", exactTerm),
			)
		}

		disMaxQueries := []elastic.Query{
			// Language analyzed, exact (no slop)
			elastic.NewMatchPhraseQuery("title.language", exactTerm).Boost(EXACT_BOOST * TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title.language", exactTerm).Boost(EXACT_BOOST * FULL_TITLE_BOOST),
			// Standard analyzed, exact (no slop).
			elastic.NewMatchPhraseQuery("title", exactTerm).Boost(STANDARD_BOOST * EXACT_BOOST * TITLE_BOOST),
			elastic.NewMatchPhraseQuery("full_title", exactTerm).Boost(STANDARD_BOOST * EXACT_BOOST * FULL_TITLE_BOOST),
		}
		if appendDecription {
			disMaxQueries = append(disMaxQueries,
				// Language analyzed, exact (no slop)
				elastic.NewMatchPhraseQuery("description.language", exactTerm).Boost(EXACT_BOOST*DESCRIPTION_BOOST),
				// Standard analyzed, exact (no slop).
				elastic.NewMatchPhraseQuery("description", exactTerm).Boost(STANDARD_BOOST*EXACT_BOOST*DESCRIPTION_BOOST),
			)
		}
		if !titlesOnly {
			disMaxQueries = append(disMaxQueries,
				// Language analyzed, exact (no slop)
				elastic.NewMatchPhraseQuery("content.language", exactTerm).Boost(EXACT_BOOST),
				// Standard analyzed, exact (no slop).
				elastic.NewMatchPhraseQuery("content", exactTerm).Boost(STANDARD_BOOST*EXACT_BOOST),
			)
		}

		boolQuery = boolQuery.Must(
			// Don't calculate score here, as we use sloped score below.
			elastic.NewConstantScoreQuery(
				elastic.NewBoolQuery().Should(constantScoreQueries...).MinimumNumberShouldMatch(1),
			).Boost(0.0),
		).Should(
			elastic.NewDisMaxQuery().Query(disMaxQueries...),
		)
	}
	contentTypeQuery := elastic.NewBoolQuery().MinimumNumberShouldMatch(1)
	filterByContentType := false
	for filter, values := range q.Filters {
		s := make([]string, len(values))
		for i, v := range values {
			s[i] = v
		}
		switch filter {
		case consts.FILTERS[consts.FILTER_START_DATE]:
			boolQuery.Filter(elastic.NewRangeQuery("effective_date").Gte(values[0]).Format("yyyy-MM-dd"))
		case consts.FILTERS[consts.FILTER_END_DATE]:
			boolQuery.Filter(elastic.NewRangeQuery("effective_date").Lte(values[0]).Format("yyyy-MM-dd"))
		case consts.FILTERS[consts.FILTER_UNITS_CONTENT_TYPES], consts.FILTERS[consts.FILTER_COLLECTIONS_CONTENT_TYPES]:
			contentTypeQuery.Should(elastic.NewTermsQuery("filter_values", es.KeyIValues(filter, s)...))
			filterByContentType = true
		case consts.FILTERS[consts.FILTER_SECTION_SOURCES]:
			boolQuery.Filter(elastic.NewTermsQuery("result_type", consts.ES_RESULT_TYPE_SOURCES))
		default:
			boolQuery.Filter(elastic.NewTermsQuery("filter_values", es.KeyIValues(filter, s)...))
		}
		if filterByContentType {
			boolQuery.Filter(contentTypeQuery)
		}
	}

	var query elastic.Query
	query = boolQuery

	if q.Term == "" && len(q.ExactTerms) == 0 {
		// No potential score from string matching.
		query = elastic.NewConstantScoreQuery(boolQuery).Boost(1.0)
	}
	scoreQuery := elastic.NewFunctionScoreQuery().ScoreMode("multiply")
	for _, resultType := range resultTypes {
		weight := 1.0
		if resultType == consts.ES_RESULT_TYPE_UNITS {
			weight = 1.1
		} else if resultType == consts.ES_RESULT_TYPE_TAGS {
			weight = 2.3 // We use tags for intents only
		} else if resultType == consts.ES_RESULT_TYPE_SOURCES {
			weight = 1.8
		} else if resultType == consts.ES_RESULT_TYPE_COLLECTIONS {
			weight = 2.0
		}
		scoreQuery.Add(elastic.NewTermsQuery("result_type", resultType), elastic.NewWeightFactorFunction(weight))
	}

	// Reduce score for clips.
	scoreQuery.Add(elastic.NewTermsQuery("filter_values", es.KeyValue("content_type", consts.CT_CLIP)), elastic.NewWeightFactorFunction(0.7))
	return elastic.NewFunctionScoreQuery().Query(scoreQuery.Query(query).MinScore(MIN_SCORE_FOR_RESULTS)).ScoreMode("sum").MaxBoost(100.0).
		AddScoreFunc(elastic.NewWeightFactorFunction(2.0)).
		AddScoreFunc(elastic.NewGaussDecayFunction().FieldName("effective_date").Decay(0.6).Scale("2000d")), nil
}

func NewResultsSearchRequest(options SearchRequestOptions) (*elastic.SearchRequest, error) {
	fetchSourceContext := elastic.NewFetchSourceContext(true).Include("mdb_uid", "result_type", "effective_date")

	titleAdded := false
	fullTitleAdded := false
	contentAdded := false
	typedUidsAdded := false
	//	This is a generic imp. that supports searching tweets together with other results.
	//	Currently we are not searching for tweets together with other results but in parallel.
	for _, rt := range options.resultTypes {
		if rt == consts.ES_RESULT_TYPE_COLLECTIONS {
			fetchSourceContext.Include("typed_uids")
		}
		if options.includeTypedUidsFromContentUnits && rt == consts.ES_RESULT_TYPE_UNITS && !typedUidsAdded {
			fetchSourceContext.Include("typed_uids")
			typedUidsAdded = true
		} else if rt == consts.ES_RESULT_TYPE_TWEETS && !contentAdded {
			fetchSourceContext.Include("content")
			contentAdded = true
		} else if rt == consts.ES_RESULT_TYPE_SOURCES && !fullTitleAdded {
			fetchSourceContext.Include("full_title")
			fullTitleAdded = true
		}
		if !titleAdded && rt != consts.ES_RESULT_TYPE_TWEETS {
			fetchSourceContext.Include("title")
			titleAdded = true
		}
		if contentAdded && titleAdded && fullTitleAdded && (!options.includeTypedUidsFromContentUnits || typedUidsAdded) {
			break
		}
	}

	resultsQuery, err := createResultsQuery(options.resultTypes, options.query, options.docIds, options.filterOutCUSources, options.titlesOnly)
	if err != nil {
		fmt.Printf("Error creating results query: %s", err.Error())
		return nil, err
	}

	source := elastic.NewSearchSource().
		Query(resultsQuery).
		FetchSourceContext(fetchSourceContext).
		From(options.from).
		Size(options.size).
		Explain(options.query.Deb)

	if options.Timeout != nil {
		source = source.Timeout(*options.Timeout)
	}

	if options.useHighlight {
		terms := make([]string, 1)
		if options.query.Term != "" {
			terms = append(terms, options.query.Term)
		} else {
			terms = options.query.ExactTerms
		}

		contentNumOfFragments := 5 //  elastic default
		if options.highlightFullContent {
			contentNumOfFragments = 0
		}
		highlightQuery := createHighlightQuery(terms, contentNumOfFragments, options.partialHighlight)

		source = source.Highlight(highlightQuery)
	}

	switch options.sortBy {
	case consts.SORT_BY_OLDER_TO_NEWER:
		source = source.Sort("effective_date", true)
	case consts.SORT_BY_NEWER_TO_OLDER:
		source = source.Sort("effective_date", false)
	}
	return elastic.NewSearchRequest().
		SearchSource(source).
		Index(options.index).
		Preference(options.preference), nil
}

func createHighlightQuery(terms []string, n int, partialHighlight bool) *elastic.Highlight {
	//  We use special HighlightQuery with SimpleQueryStringQuery to
	//	 solve elastic issue with synonyms and highlights.

	query := elastic.NewHighlight()
	for _, term := range terms {
		query.Fields(
			elastic.NewHighlighterField("title").NumOfFragments(0).HighlightQuery(elastic.NewSimpleQueryStringQuery(term)),
			elastic.NewHighlighterField("full_title").NumOfFragments(0).HighlightQuery(elastic.NewSimpleQueryStringQuery(term)),
			elastic.NewHighlighterField("description").HighlightQuery(elastic.NewSimpleQueryStringQuery(term)),
			elastic.NewHighlighterField("description.language").HighlightQuery(elastic.NewSimpleQueryStringQuery(term)),
			elastic.NewHighlighterField("content").NumOfFragments(n).HighlightQuery(elastic.NewSimpleQueryStringQuery(term)),
			elastic.NewHighlighterField("content.language").NumOfFragments(n).HighlightQuery(elastic.NewSimpleQueryStringQuery(term)))

		if !partialHighlight {
			// Following field not used in intents to solve elastic bug with highlight.
			query.Fields(
				elastic.NewHighlighterField("title.language").NumOfFragments(0).HighlightQuery(elastic.NewSimpleQueryStringQuery(term)))
		}
	}
	return query
}

func NewResultsSearchRequests(options SearchRequestOptions) ([]*elastic.SearchRequest, error) {
	requests := make([]*elastic.SearchRequest, 0)
	indices := make([]string, len(options.query.LanguageOrder))
	for i := range options.query.LanguageOrder {
		indices[i] = es.IndexNameForServing("prod", consts.ES_RESULTS_INDEX, options.query.LanguageOrder[i])
	}
	for _, index := range indices {
		options.index = index
		request, err := NewResultsSearchRequest(options)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func NewResultsSuggestRequest(resultTypes []string, index string, query Query, preference string) *elastic.SearchRequest {
	fetchSourceContext := elastic.NewFetchSourceContext(true).Include("mdb_uid", "result_type", "title", "full_title")
	searchSource := elastic.NewSearchSource().
		FetchSourceContext(fetchSourceContext).
		Suggester(
			elastic.NewCompletionSuggester("title_suggest").
				Field("title_suggest").
				Text(query.Term).
				ContextQuery(elastic.NewSuggesterCategoryQuery("result_type", resultTypes...)).
				Size(NUM_SUGGESTS).
				SkipDuplicates(true)).
		Suggester(
			elastic.NewCompletionSuggester("title_suggest.language").
				Field("title_suggest.language").
				Text(query.Term).
				ContextQuery(elastic.NewSuggesterCategoryQuery("result_type", resultTypes...)).
				Size(NUM_SUGGESTS).
				SkipDuplicates(true))

	return elastic.NewSearchRequest().
		SearchSource(searchSource).
		Index(index).
		Preference(preference)
}

func NewResultsSuggestRequests(resultTypes []string, query Query, preference string) []*elastic.SearchRequest {
	requests := make([]*elastic.SearchRequest, 0)
	indices := make([]string, len(query.LanguageOrder))
	for i := range query.LanguageOrder {
		indices[i] = es.IndexNameForServing("prod", consts.ES_RESULTS_INDEX, query.LanguageOrder[i])
	}
	for _, index := range indices {
		request := NewResultsSuggestRequest(resultTypes, index, query, preference)
		requests = append(requests, request)
	}
	return requests
}
