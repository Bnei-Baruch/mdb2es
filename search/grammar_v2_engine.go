package search

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/Bnei-Baruch/archive-backend/mdb"

	log "github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"gopkg.in/olivere/elastic.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

func (e *ESEngine) SuggestGrammarsV2(query *Query, preference string) (map[string][]VariablesByPhrase, error) {
	start := time.Now()
	// Map from lang => Original Full Phrase => $Var => values
	suggests := make(map[string][]VariablesByPhrase)

	if query.Term == "" || len(query.ExactTerms) > 0 {
		log.Infof("Term is empty of exact term exists, should not trigger grammar: [%s] [%s]", query.Term, strings.Join(query.ExactTerms, " - "))
		return suggests, nil
	}

	multiSearchService := e.esc.MultiSearch()

	for _, language := range query.LanguageOrder {
		// Suggester:
		multiSearchService.Add(NewResultsSuggestGrammarV2CompletionRequest(query, language, preference))
		// Search (will not match part of words): multiSearchService.Add(NewSuggestGammarV2Request(query, language, preference))
	}

	mr, err := multiSearchService.Do(context.TODO())
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		fmt.Printf("multiSearchService.Do - %s\n\n", elapsed.String())
	}
	if err != nil {
		return nil, errors.Wrap(err, "Error looking for grammar suggest.")
	}

	if len(mr.Responses) != len(query.LanguageOrder) {
		return nil, errors.New(fmt.Sprintf("Unexpected number of results %d, expected %d",
			len(mr.Responses), len(query.LanguageOrder)))
	}

	start = time.Now()
	for i, currentResults := range mr.Responses {
		if currentResults.Error != nil {
			log.Warnf("%+v", currentResults.Error)
			return nil, errors.New(fmt.Sprintf("Failed multi get: %+v", currentResults.Error))
		}
		// Suggester
		if SuggestionHasOptions(currentResults.Suggest) {
			language := query.LanguageOrder[i]
			if suggests[language], err = e.suggestOptionsToVariablesByPhrases(query, &currentResults.Suggest); err != nil {
				return nil, err
			}
		}

		// Searcher: <=== will not match part of words.
		//if haveHits(currentResults) {
		//	language := query.LanguageOrder[i]
		//	if suggests[language], err = e.suggestResultsToVariablesByPhrases(query, currentResults); err != nil {
		//		return nil, err
		//	}
		//}

	}
	elapsed = time.Since(start)
	if elapsed > 10*time.Millisecond {
		fmt.Printf("build suggests - %s\n\n", elapsed.String())
	}

	return suggests, nil
}

func (e *ESEngine) suggestOptionsToVariablesByPhrases(query *Query, suggest *elastic.SearchSuggest) ([]VariablesByPhrase, error) {
	ret := []VariablesByPhrase(nil)
	for _, v := range *suggest {
		for _, s := range v {
			if len(s.Options) > 0 {
				for _, option := range s.Options {
					var ruleObj GrammarRuleWithPercolatorQuery
					if err := json.Unmarshal(*option.Source, &ruleObj); err != nil {
						return nil, err
					}
					rule := ruleObj.GrammarRule
					// log.Infof("Score: %.2f, Index: %s, Type: %s, Id: %s, Source: %+v", option.Score, option.Index, option.Type, option.Id, rule)
					if len(rule.Values) != len(rule.Variables) {
						return nil, errors.New(fmt.Sprintf("Expected Variables to be of size %d, but it is %d", len(rule.Values), len(rule.Variables)))
					}
					vMap := make(map[string][]string)
					for i := range rule.Variables {
						vMap[rule.Variables[i]] = []string{rule.Values[i]}
					}
					if GrammarVariablesMatch(rule.Intent, vMap, e.cache) {
						//log.Infof("Chosen: %s", chosen)
						//log.Infof("Score: %.2f, Index: %s, Type: %s, Id: %s, Source: %+v", option.Score, option.Index, option.Type, option.Id, rule)
						//log.Infof("Options: %+v", option)
						//log.Infof("vMap: [%+v]", vMap)
						// Map from lang => Original Full Phrase => $Var => values
						variablesByPhrase := make(VariablesByPhrase)
						variablesByPhrase[chooseRule(query, rule.Rules)] = vMap
						ret = append(ret, variablesByPhrase)
					}
				}
			}
		}
	}

	return ret, nil
}

func (e *ESEngine) suggestResultsToVariablesByPhrases(query *Query, result *elastic.SearchResult) ([]VariablesByPhrase, error) {
	ret := []VariablesByPhrase(nil)
	if haveHits(result) {
		// log.Infof("Total Hits: %d, Max Score: %.2f", result.Hits.TotalHits, *result.Hits.MaxScore)
		for _, hit := range result.Hits.Hits {
			var rule GrammarRule
			if err := json.Unmarshal(*hit.Source, &rule); err != nil {
				return nil, err
			}
			// log.Infof("Score: %.2f, Index: %s, Type: %s, Id: %s, Source: %+v", *hit.Score, hit.Index, hit.Type, hit.Id, rule)
			if len(rule.Values) != len(rule.Variables) {
				return nil, errors.New(fmt.Sprintf("Expected Variables to be of size %d, but it is %d", len(rule.Values), len(rule.Variables)))
			}
			vMap := make(map[string][]string)
			for i := range rule.Variables {
				vMap[rule.Variables[i]] = []string{rule.Values[i]}
			}
			if GrammarVariablesMatch(rule.Intent, vMap, e.cache) {
				// Map from lang => Original Full Phrase => $Var => values
				variablesByPhrase := make(VariablesByPhrase)
				variablesByPhrase[chooseRule(query, rule.Rules)] = vMap
				ret = append(ret, variablesByPhrase)
			}
		}
	}
	return ret, nil
}

// Return: single hit intents, filtering intents
func (e *ESEngine) SearchGrammarsV2(query *Query, from int, size int, sortBy string, resultTypes []string, preference string) ([]Intent, []Intent, error) {
	singleHitIntents := []Intent{}
	filterIntents := []Intent{}
	if query.Term != "" && len(query.ExactTerms) > 0 {
		// Will never match any grammar for query having simple terms and exact terms.
		// This is not acurate but an edge case. Need to better think of query representation.
		log.Infof("Both term and exact terms are defined, should not trigger: [%s] [%s]", query.Term, strings.Join(query.ExactTerms, " - "))
		return singleHitIntents, filterIntents, nil
	}
	if e.cache != nil && e.cache.SearchStats().DoesSourceTitleWithMoreThanOneWordExist(query.Term) {
		// Since some source titles contains grammar variable values,
		// we are not triggering grammar search if the term eqauls to a title of a source.
		// Some examples for such source titles:
		// 'Book, Author, Story','Connecting to the Source', 'Introduction to articles', 'שיעור ההתגברות', 'ספר הזוהר'
		log.Infof("The term is identical to a title of a source, should not trigger: [%s]", query.Term)
		return singleHitIntents, filterIntents, nil
	}

	multiSearchService := e.esc.MultiSearch()
	for _, language := range query.LanguageOrder {
		multiSearchService.Add(NewSuggestGammarV2Request(query, language, preference))
		multiSearchService.Add(NewGammarPerculateRequest(query, language, preference))
	}
	beforeGrammarSearch := time.Now()
	mr, err := multiSearchService.Do(context.TODO())
	e.timeTrack(beforeGrammarSearch, consts.LAT_DOSEARCH_GRAMMARS_MULTISEARCHGRAMMARSDO)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Error looking for grammar search.")
	}

	if len(mr.Responses) != len(query.LanguageOrder)*2 {
		return nil, nil, errors.New(fmt.Sprintf("Unexpected number of results %d, expected %d",
			len(mr.Responses), len(query.LanguageOrder)*2))
	}

	start := time.Now()
	for i, currentResults := range mr.Responses {
		if currentResults.Error != nil {
			log.Warnf("%+v", currentResults.Error)
			return nil, nil, errors.New(fmt.Sprintf("Failed multi get: %+v", currentResults.Error))
		}
		language := query.LanguageOrder[i/2]
		if haveHits(currentResults) {
			if languageSingleHitIntents, languageFilterIntents, err := e.searchResultsToIntents(query, language, currentResults); err != nil {
				return nil, nil, err
			} else {
				singleHitIntents = append(singleHitIntents, languageSingleHitIntents...)
				if languageFilterIntents != nil {
					if len(languageFilterIntents) > 1 {
						return singleHitIntents, nil, errors.Errorf("Number of filter intents for language '%v' is %v but only 1 filter intent is currently supported.", language, len(filterIntents))
					} else if len(languageFilterIntents) == 1 {
						filterIntents = append(filterIntents, languageFilterIntents...)
					}
				}
			}
		}
	}
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		fmt.Printf("build grammar intent - %s\n\n", elapsed.String())
	}
	return singleHitIntents, filterIntents, nil
}

// Search according to grammar based filter (currently by content types and free text).
func (e *ESEngine) SearchByFilterIntents(filterIntents []Intent, originalSearchTerm string, from int, size int, sortBy string, resultTypes []string, preference string, deb bool) (map[string]FilteredSearchResult, error) {
	resultsByLang := map[string]FilteredSearchResult{}
	for _, intent := range filterIntents {
		if intentValue, ok := filterIntents[0].Value.(GrammarIntent); ok {
			var contentType string
			var text string
			sources := []string{}
			for _, fv := range intentValue.FilterValues {
				if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_CONTENT_TYPE] {
					contentType = fv.Value
				} else if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_TEXT] {
					text = fv.Value
				} else if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_SOURCE] {
					sources = append(sources, fv.Value)
				}
			}
			if text != "" && (contentType != "" || len(sources) > 0) {
				log.Infof("Filtered Search Request: ContentType is '%s', Text is '%s', Sources are '%+v'.", contentType, text, sources)
				requests := []*elastic.SearchRequest{}
				textValSearchRequests, err := NewFilteredResultsSearchRequest(text, contentType, sources, from, size, sortBy, resultTypes, intent.Language, preference, deb)
				if err != nil {
					return nil, err
				}
				requests = append(requests, textValSearchRequests...)
				if contentType != consts.VAR_CT_ARTICLES {
					fullTermSearchRequests, err := NewFilteredResultsSearchRequest(originalSearchTerm, contentType, sources, from, size, sortBy, resultTypes, intent.Language, preference, deb)
					if err != nil {
						return nil, err
					}
					requests = append(requests, fullTermSearchRequests...)
				}
				if len(requests) > 0 {
					// All search requests here are for the same language
					results, hitIdsMap, maxScore, err := e.filterSearch(requests)
					if err != nil {
						return nil, err
					}
					resultsByLang[intent.Language] = FilteredSearchResult{
						Results:     results,
						Term:        text,
						ContentType: contentType,
						HitIdsMap:   hitIdsMap,
						MaxScore:    maxScore,
					}
					if len(results) > 0 {
						// we assume that there is no need to make the search for other languages if a results found for one language
						break
					}
				}
			}
		} else {
			return nil, errors.Errorf("FilterSearch error. Intent is not GrammarIntent. Intent: %+v", intent)
		}
	}
	return resultsByLang, nil
}

func (e *ESEngine) VariableMapToFilterValues(vMap map[string][]string, language string) []FilterValue {
	ret := []FilterValue{}
	for name, values := range vMap {
		// TODO: Actually map the variable names to filter names and variable values to filter values.
		// Maybe this should be done in frontend...
		filterName, ok := consts.VARIABLE_TO_FILTER[name]
		if !ok {
			filterName = name
		}
		for _, value := range values {
			var origin string
			if len(e.variables[name][language][value]) > 0 {
				//  we store 'origin' only for variables with a finite values list
				origin = e.variables[name][language][value][0]
			}
			ret = append(ret, FilterValue{
				Name:       filterName,
				Value:      value,
				Origin:     origin,
				OriginFull: origin,
			})
		}
	}
	return ret
}

// For specific landing page, keep only some amount of intents with the highest score (according to MAX_MATCHES_PER_GRAMMAR_INTENT) and filter out all the rest.
// Return the minimum score of the intents slice for the given intent landing page.
func updateIntentCount(intentsCount map[string][]Intent, intent Intent) float64 {
	var minScore float64
	intents := intentsCount[intent.Value.(GrammarIntent).LandingPage]
	if len(intents) > 0 {
		lastElem := intents[len(intents)-1]
		minScore = lastElem.Value.(GrammarIntent).Score
	}
	if intent.Value.(GrammarIntent).SingleCollectionMdbUid != nil {
		for _, i := range intents {
			if i.Value.(GrammarIntent).SingleCollectionMdbUid != nil &&
				*i.Value.(GrammarIntent).SingleCollectionMdbUid == *intent.Value.(GrammarIntent).SingleCollectionMdbUid {
				// Ignore duplicate collections
				return minScore
			}
		}
	}
	intents = append(intents, intent)
	sort.SliceStable(intents, func(i, j int) bool {
		return intents[i].Value.(GrammarIntent).Score > intents[j].Value.(GrammarIntent).Score
	})
	intents = intents[:utils.MinInt(consts.MAX_MATCHES_PER_GRAMMAR_INTENT, len(intents))]
	if len(intents) > 0 {
		lastElem := intents[len(intents)-1]
		minScore = lastElem.Value.(GrammarIntent).Score
	}
	intentsCount[intent.Value.(GrammarIntent).LandingPage] = intents
	return minScore
}

// Return values: singleHitIntents, filterIntents, error
func (e *ESEngine) searchResultsToIntents(query *Query, language string, result *elastic.SearchResult) ([]Intent, []Intent, error) {
	// log.Infof("Total Hits: %d, Max Score: %.2f", result.Hits.TotalHits, *result.Hits.MaxScore)
	filterIntents := []Intent(nil)
	singleHitIntents := []Intent(nil)
	intentsCount := make(map[string][]Intent)
	minScoreByLandingPage := make(map[string]float64)
	for _, hit := range result.Hits.Hits {
		var ruleObj GrammarRuleWithPercolatorQuery
		if err := json.Unmarshal(*hit.Source, &ruleObj); err != nil {
			return nil, nil, err
		}
		rule := ruleObj.GrammarRule
		// log.Infof("Score: %.2f, Index: %s, Type: %s, Id: %s, Source: %+v", *hit.Score, hit.Index, hit.Type, hit.Id, rule)
		if len(rule.Values) != len(rule.Variables) {
			return nil, nil, errors.New(fmt.Sprintf("Expected Variables to be of size %d, but it is %d", len(rule.Values), len(rule.Variables)))
		}

		// Check filters match, i.e., existing query filter match at least one supported grammar intent filter.
		if len(query.Filters) > 0 {
			filters, filterExist := consts.GRAMMAR_INTENTS_TO_FILTER_VALUES[rule.Intent]
			if !filterExist {
				return nil, nil, errors.New(fmt.Sprintf("Filters not found for intent: [%s]", rule.Intent))
			}
			common := false
			for filterName, values := range query.Filters {
				sort.Strings(values)
				if grammarValues, ok := filters[filterName]; ok {
					sort.Strings(grammarValues)
					if len(utils.IntersectSortedStringSlices(values, grammarValues)) > 0 {
						common = true
						break
					}
				}
			}
			if !common {
				// No matching filter found, should not trigger intent.
				log.Infof("No common filters for intent [%s]: %+v vs %+v", rule.Intent, query.Filters, filters)
				continue
			}
		}

		vMap := make(map[string][]string)
		for i := range rule.Variables {
			if rule.Variables[i] == consts.VAR_TEXT {
				if hit.Highlight != nil {
					if text, ok := hit.Highlight["search_text"]; ok {
						log.Infof("search_text: %s", text)
						if len(text) == 1 && text[0] != "" {
							textVarValues := retrieveTextVarValues(text[0])
							vMap[rule.Variables[i]] = textVarValues
							log.Infof("$Text values are %+v", textVarValues)
						}
					}
				}
			} else {
				vMap[rule.Variables[i]] = []string{rule.Values[i]}
			}
		}

		if GrammarVariablesMatch(rule.Intent, vMap, e.cache) {
			score := *hit.Score * (float64(4) / float64(4+len(vMap))) * YearScorePenalty(vMap)
			// Issue with tf/idf. For query [congress] the score if very low. For [arava] ok.
			// Fix this by moving the grammar index into the common index. So tha similar tf/idf will be used.
			// For now solve by normalizing very small scores.
			// log.Infof("Intent: %+v score: %.2f %.2f %.2f", vMap, *hit.Score, (float64(4) / float64(4+len(vMap))), score)
			if rule.Intent == consts.GRAMMAR_INTENT_FILTER_BY_CONTENT_TYPE || rule.Intent == consts.GRAMMAR_INTENT_FILTER_BY_SOURCE {
				filterIntents = append(filterIntents, Intent{
					Type:     consts.GRAMMAR_TYPE_FILTER,
					Language: language,
					Value: GrammarIntent{
						FilterValues: e.VariableMapToFilterValues(vMap, language),
						Score:        score,
						Explanation:  hit.Explanation,
					}})
			} else if rule.Intent == consts.GRAMMAR_INTENT_CLASSIFICATION_BY_CONTENT_TYPE_AND_SOURCE {
				var contentType string
				var source string
				filterValues := e.VariableMapToFilterValues(vMap, language)
				for _, fv := range filterValues {
					if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_CONTENT_TYPE] {
						contentType = fv.Value
					} else if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_SOURCE] {
						source = fv.Value
					}
					if contentType != "" && source != "" {
						break
					}
				}
				if opt, ok := consts.INTENT_OPTIONS_BY_GRAMMAR_CT_VARIABLES[contentType]; ok {
					for _, cut := range opt.ContentTypes {
						//var score float64 = 3000 //TBC
						ci := ClassificationIntent{
							ResultType:  consts.ES_RESULT_TYPE_SOURCES,
							MDB_UID:     source,
							Title:       query.Term,
							ContentType: cut,
							Exist:       e.cache.SearchStats().IsSourceWithEnoughUnits(source, consts.INTENTS_MIN_UNITS, cut),
							Score:       &score,
						}
						intent := Intent{consts.INTENT_TYPE_SOURCE, language, ci}
						singleHitIntents = append(singleHitIntents, intent)
					}
				}
			} else {
				if intentsByLandingPage, ok := intentsCount[rule.Intent]; ok && len(intentsByLandingPage) >= consts.MAX_MATCHES_PER_GRAMMAR_INTENT {
					if score <= minScoreByLandingPage[rule.Intent] {
						// Initial filtering (before updateIntentCount func.) to avoid the SQL call for converting LP to collection.
						continue
					}
				}
				intentValue := GrammarIntent{
					LandingPage:  rule.Intent,
					FilterValues: e.VariableMapToFilterValues(vMap, language),
					Score:        score,
					Explanation:  hit.Explanation,
				}
				if intentValue.LandingPage == consts.GRAMMAR_INTENT_LANDING_PAGE_CONVENTIONS && intentValue.FilterValues != nil {
					var year string
					var location string
					for _, fv := range intentValue.FilterValues {
						if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_CONVENTION_LOCATION] {
							location = fv.Value

						} else if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_YEAR] {
							year = fv.Value
						}
						if year != "" && location != "" {
							break
						}
					}
					if e.cache.SearchStats().DoesConventionSingle(location, year) {
						// Since the LandingPage has only one collection item, convert the LandingPage result to the single collection hit
						log.Infof("Converting LandingPage of %s %s to a single collection.", location, year)
						var err error
						collectionHit, mdbUid, err := e.conventionsLandingPageToCollectionHit(year, location)
						if err != nil {
							log.Warnf("%+v", err)
							return nil, nil, errors.New(fmt.Sprintf("ConventionsLandingPageToCollectionHit Failed: %+v", err))
						}
						intentValue.SingleCollection = collectionHit
						intentValue.SingleCollectionMdbUid = mdbUid
					}
				}
				if intentValue.LandingPage == consts.GRAMMAR_INTENT_LANDING_PAGE_HOLIDAYS && intentValue.FilterValues != nil {
					var year string
					var holiday string
					for _, fv := range intentValue.FilterValues {
						if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_HOLIDAYS] {
							holiday = fv.Value

						} else if fv.Name == consts.VARIABLE_TO_FILTER[consts.VAR_YEAR] {
							year = fv.Value
						}
						if year != "" && holiday != "" {
							break
						}
					}
					if e.cache.SearchStats().DoesHolidaySingle(holiday, year) {
						// Since the LandingPage has only one collection item, convert the LandingPage result to the single collection hit
						log.Infof("Converting LandingPage of %s %s to a single collection.", holiday, year)
						var err error
						collectionHit, mdbUid, err := e.holidaysLandingPageToCollectionHit(year, holiday)
						if err != nil {
							log.Warnf("%+v", err)
							return nil, nil, errors.New(fmt.Sprintf("HolidaysLandingPageToCollectionHit Failed: %+v", err))
						}
						intentValue.SingleCollection = collectionHit
						intentValue.SingleCollectionMdbUid = mdbUid
					}
				}
				intent := Intent{
					Type:     consts.GRAMMAR_TYPE_LANDING_PAGE,
					Language: language,
					Value:    intentValue,
				}
				minScoreByLandingPage[intent.Value.(GrammarIntent).LandingPage] = updateIntentCount(intentsCount, intent)
			}
		}
	}
	for _, intentsByLandingPage := range intentsCount {
		singleHitIntents = append(singleHitIntents, intentsByLandingPage...)
	}

	// Normalize score to be from 2000 and below.
	maxScore := 0.0
	for i := range singleHitIntents {
		if singleHitIntents[i].Value.(GrammarIntent).Score > maxScore {
			maxScore = singleHitIntents[i].Value.(GrammarIntent).Score
		}
	}
	normalizedLandingPageIntents := []Intent(nil)
	for _, intent := range singleHitIntents {
		grammarIntent := intent.Value.(GrammarIntent)
		grammarIntent.Score = 3000 * (grammarIntent.Score / maxScore)
		intent.Value = grammarIntent
		normalizedLandingPageIntents = append(normalizedLandingPageIntents, intent)
	}
	//log.Infof("landingPageIntents: %+v", normalizedLandingPageIntents)
	//log.Infof("filterIntents: %+v", filterIntents)
	return normalizedLandingPageIntents, filterIntents, nil
}

func (e *ESEngine) conventionsLandingPageToCollectionHit(year string, location string) (*elastic.SearchHit, *string, error) {
	queryMask := `select c.uid, c.properties from collections c 
	where c.type_id=%d
	%s`
	cityMask := `c.properties ->> 'city' = '%s'`
	countryMask := `c.properties ->> 'country' = '%s'`
	yearMask := `extract(year from (c.properties ->> 'start_date')::date) = %s`

	var country string
	var city string

	if location != "" {
		s := strings.Split(location, "|")
		country = s[0]
		if len(s) > 1 {
			city = s[1]
		}
	}

	whereClauses := make([]string, 0)
	if year != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(yearMask, year))
	}
	if country != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(countryMask, country))
	}
	if city != "" {
		whereClauses = append(whereClauses, fmt.Sprintf(cityMask, city))
	}

	var whereQuery string
	if len(whereClauses) > 0 {
		whereQuery = fmt.Sprintf("and %s", strings.Join(whereClauses, " and "))
	}
	query := fmt.Sprintf(queryMask, mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_CONGRESS].ID, whereQuery)
	//log.Infof("ConventionsLandingPageToCollectionHit Query: %s", query)
	return e.collectionHitFromSql(query)
}

func (e *ESEngine) holidaysLandingPageToCollectionHit(year string, holiday string) (*elastic.SearchHit, *string, error) {
	queryMask := `select c.uid, c.properties from collections c
	join tags t on c.properties ->> 'holiday_tag' = t.uid
	%s`
	uidMask := `t.uid = '%s'`
	yearMask := `(extract(year from (c.properties ->> 'start_date')::date) = %s or extract(year from (c.properties ->> 'end_date')::date) = %s)`

	var whereQuery string
	if year != "" && holiday != "" {
		whereQuery = fmt.Sprintf("where %s and %s", fmt.Sprintf(uidMask, holiday), fmt.Sprintf(yearMask, year, year))
	} else if year != "" {
		whereQuery = fmt.Sprintf("where %s", fmt.Sprintf(yearMask, year, year))
	} else if holiday != "" {
		whereQuery = fmt.Sprintf("where %s", fmt.Sprintf(uidMask, holiday))
	}

	query := fmt.Sprintf(queryMask, whereQuery)
	//log.Infof("QUERY: %s", query)
	return e.collectionHitFromSql(query)
}

func (e *ESEngine) collectionHitFromSql(query string) (*elastic.SearchHit, *string, error) {
	var properties json.RawMessage
	var mdbUID string
	var effectiveDate es.EffectiveDate

	err := e.mdb.QueryRow(query).Scan(&mdbUID, &properties)
	if err != nil {
		return nil, nil, err
	}

	err = json.Unmarshal(properties, &effectiveDate)
	if err != nil {
		return nil, nil, err
	}

	result := es.Result{
		EffectiveDate: effectiveDate.EffectiveDate,
		MDB_UID:       mdbUID,
		ResultType:    consts.ES_RESULT_TYPE_COLLECTIONS,
	}

	resultJson, err := json.Marshal(result)
	if err != nil {
		return nil, nil, err
	}

	hit := &elastic.SearchHit{
		Source: (*json.RawMessage)(&resultJson),
		Type:   "result",
		Index:  consts.GRAMMAR_LP_SINGLE_COLLECTION,
	}
	return hit, &mdbUID, nil
}

// This function retrieves the 'free text' values from a grammar result that was searched by perculator query with highlight.
// The 'highlighted' part of the input string contains the values that are NOT 'free text'. This parts starts and ends with PERCULATE_HIGHLIGHT_SEPERATOR rune ('$').
// The return value of the function is a slice of all term parts thar are outside of the 'highlight'.
// For example, the 'free text' values for the term 'aaa $bbb$ ccc $ddd' are 'aaa' and 'ccc'.
// We have a test for this function in engine_test.go
func retrieveTextVarValues(str string) []string {
	runes := []rune(str)
	var filtered []rune
	var textVarValues []string
	var inHighlight bool
	for i, r := range runes {
		if r == PERCULATE_HIGHLIGHT_SEPERATOR || i == len(runes)-1 {
			inHighlight = !inHighlight
			if inHighlight && len(filtered) > 0 {
				if r != PERCULATE_HIGHLIGHT_SEPERATOR {
					filtered = append(filtered, r)
				}
				trimmed := strings.Trim(string(filtered), " ")
				if trimmed != "" {
					textVarValues = append(textVarValues, trimmed)
				}
			}
			filtered = make([]rune, 0)
		} else if !inHighlight {
			filtered = append(filtered, r)
		}
	}
	return textVarValues
}

// Results search according to grammar based filter (currently by content types and free text).
// Return: Results, Unique list of hit id's as a map, Max score
func (e *ESEngine) filterSearch(requests []*elastic.SearchRequest) ([]*elastic.SearchResult, map[string]bool, *float64, error) {
	results := []*elastic.SearchResult{}
	hitIdsMap := map[string]bool{}
	var maxScore *float64

	multiSearchFilteredService := e.esc.MultiSearch()
	multiSearchFilteredService.Add(requests...)
	beforeFilterSearch := time.Now()
	mr, err := multiSearchFilteredService.Do(context.TODO())
	e.timeTrack(beforeFilterSearch, consts.LAT_DOSEARCH_GRAMMARS_MULTISEARCHGRAMMARSDO)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Error looking for grammar based filter search.")
	}

	for _, currentResults := range mr.Responses {
		if currentResults.Error != nil {
			log.Warnf("%+v", currentResults.Error)
			return nil, nil, nil, errors.New(fmt.Sprintf("Failed multi get in grammar based filter search: %+v", currentResults.Error))
		}
		if haveHits(currentResults) {
			if currentResults.Hits.MaxScore != nil &&
				(maxScore == nil || *currentResults.Hits.MaxScore > *maxScore) {
				maxScore = currentResults.Hits.MaxScore
			}
			for _, hit := range currentResults.Hits.Hits {
				hitIdsMap[hit.Id] = true
			}
			results = append(results, currentResults)
		}
	}
	return results, hitIdsMap, maxScore, nil
}
