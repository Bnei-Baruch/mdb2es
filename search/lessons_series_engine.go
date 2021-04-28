package search

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/es"
	"github.com/pkg/errors"
	"gopkg.in/olivere/elastic.v6"
	"strings"
	"time"
)

func (e *ESEngine) LessonsSeries(query Query, preference string) (map[string]*elastic.SearchResult, error) {
	byLang := make(map[string]*elastic.SearchResult)
	mss := e.esc.MultiSearch()

	filter := map[string][]string{consts.FILTERS[consts.FILTER_COLLECTIONS_CONTENT_TYPES]: {consts.CT_LESSONS_SERIES}}
	req, err := NewResultsSearchRequest(
		SearchRequestOptions{
			resultTypes:      []string{consts.ES_RESULT_TYPE_COLLECTIONS},
			index:            "",
			query:            Query{Term: query.Term, Filters: filter, LanguageOrder: query.LanguageOrder, Deb: query.Deb},
			sortBy:           consts.SORT_BY_RELEVANCE,
			from:             0,
			size:             100,
			preference:       preference,
			useHighlight:     false,
			partialHighlight: false})
	if err != nil {
		return nil, err
	}
	mss.Add(req)
	before := time.Now()
	mr, err := mss.Do(context.TODO())

	e.timeTrack(before, consts.LAT_DOSEARCH_MULTISEARCHTWEETSDO)
	if err != nil {
		return nil, err
	}

	for i, res := range mr.Responses {
		if res.Error != nil {
			err := errors.New(fmt.Sprintf("Failed series get: %+v", res.Error))
			return nil, err
		}
		if haveHits(res) {
			lang := query.LanguageOrder[i]
			byLang[lang] = res
		}
	}

	return combineBySource(byLang), nil
}

func combineBySource(byLang map[string]*elastic.SearchResult) map[string]*elastic.SearchResult {
	for l, r := range byLang {
		hitByS := make(map[string]*elastic.SearchHit)
		for _, h := range r.Hits.Hits {
			key := getHitSourceKey(h)
			if _, ok := hitByS[key]; !ok && key != "" {
				hitByS[key] = h
			}
		}

		byLang[l].Hits = new(elastic.SearchHits)
		for k, h := range hitByS {
			newH := &elastic.SearchHit{
				Source:      h.Source,
				Type:        consts.SEARCH_RESULT_LESSONS_SERIES,
				Score:       h.Score,
				Uid:         k,
				Explanation: h.Explanation,
			}
			if byLang[l].Hits.MaxScore == nil || *h.Score > *byLang[l].Hits.MaxScore {
				byLang[l].Hits.MaxScore = h.Score
			}
			byLang[l].Hits.TotalHits++
			byLang[l].Hits.Hits = append(byLang[l].Hits.Hits, newH)
		}
		if byLang[l].Hits == nil {
			delete(byLang, l)
		}
	}
	return byLang
}

func getHitSourceKey(hit *elastic.SearchHit) string {
	var res es.Result
	if err := json.Unmarshal(*hit.Source, &res); err != nil {
		return ""
	}
	keys := make(map[string]bool)

	if res.ResultType != consts.ES_RESULT_TYPE_COLLECTIONS {
		return ""
	}

	for _, tId := range res.TypedUids {
		l := strings.Split(tId, ":")
		if l[0] != consts.ES_UID_TYPE_SOURCE || l[1] == "" {
			continue
		}
		//clear from duplicates
		if _, ok := keys[l[1]]; !ok {
			keys[l[1]] = true
		}
	}
	var list []string
	for k, _ := range keys {
		list = append(list, k)
	}
	return strings.Join(list, "_")
}
