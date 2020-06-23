package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Bnei-Baruch/sqlboiler/queries"
	"github.com/Bnei-Baruch/sqlboiler/queries/qm"
	"github.com/lib/pq"
	"github.com/pkg/errors"
	"gopkg.in/volatiletech/null.v6"

	"github.com/Bnei-Baruch/archive-backend/consts"
	"github.com/Bnei-Baruch/archive-backend/mdb"
	mdbmodels "github.com/Bnei-Baruch/archive-backend/mdb/models"
)

type Histogram map[string]int

func (h Histogram) Increment(k string, v int) {
	h[k] += v
}

func (h Histogram) Merge(other Histogram) {
	for k, v := range other {
		h.Increment(k, v)
	}
}

type StatsNode struct {
	id       int64
	parentID int64
	uid      string
	children []int64
	hist     Histogram
}

type StatsTree struct {
	byID map[int64]*StatsNode
}

func NewStatsTree() *StatsTree {
	st := new(StatsTree)
	st.byID = make(map[int64]*StatsNode)
	return st
}

// accumulate merge Histograms bottom up so that
// parent nodes's Histogram will be the overall sum of its children.
// We do that in code because we don't really know how to do that with SQL.
func (st *StatsTree) accumulate() {
	// compute children since next step rely on it for correction
	for k, v := range st.byID {
		if v.parentID != 0 {
			parent := st.byID[v.parentID]
			parent.children = append(parent.children, k)
		}
	}

	// put all leaf nodes in s
	s := make([]int64, 0)
	for k, v := range st.byID {
		if len(v.children) == 0 {
			s = append(s, k)
		}
	}

	// while we have some nodes to merge
	for len(s) > 0 {
		// loop through this generation of nodes
		// merge parents histograms and collect next generation
		parents := make(map[int64]bool)
		for i := range s {
			node := st.byID[s[i]]
			if node.parentID != 0 {
				p := st.byID[node.parentID] // get parent
				parents[p.id] = true        // add to next gen of
				p.hist.Merge(node.hist)     // merge parent histogram with that of child
			}
		}

		// convert next generation of nodes map to slice (parents of current generation)
		s = make([]int64, len(parents))
		i := 0
		for k := range parents {
			s[i] = k
			i++
		}
	}
}

// flatten return a flat uid => Histogram lookup table.
// It's usually the only interesting result to use
// as the tree structure is not really needed once accumulated.
func (st *StatsTree) flatten() map[string]Histogram {
	byUID := make(map[string]Histogram, len(st.byID))
	for _, v := range st.byID {
		byUID[v.uid] = v.hist
	}
	return byUID
}

func (st *StatsTree) insert(id, parentID int64, uid string, ct string, cnt int) {
	node, ok := st.byID[id]
	if !ok {
		node = new(StatsNode)
		node.id = id
		node.parentID = parentID
		node.uid = uid
		node.hist = make(Histogram)
		st.byID[id] = node
	}
	if ct != "" {
		node.hist.Increment(ct, cnt)
	}
}

type ClassByTypeStats map[string]Histogram

func (s ClassByTypeStats) dump() {
	fmt.Printf("%d entries\n", len(s))
	for k, v := range s {
		fmt.Printf("%s\t\t%+v\n", k, v)
	}
}

type SearchStatsCache interface {
	Provider
	IsTagWithUnits(uid string, cts ...string) bool
	IsTagWithEnoughUnits(uid string, count int, cts ...string) bool
	IsSourceWithUnits(uid string, cts ...string) bool
	IsSourceWithEnoughUnits(uid string, count int, cts ...string) bool

	// |location| can be of: "Moscow" or "Russia|Moscow" or "Russia" or "" (empty for year constrain only)
	// |year| is 4 digit year string, e.g., "1998", "2010" or "" (empty for location constrain only)
	DoesConventionExist(location string, year string) bool
	// |holiday| is the UID of the tag that is children of 'holidays' tag
	DoesHolidayExist(holiday string, year string) bool
}

type SearchStatsCacheImpl struct {
	mdb          *sql.DB
	tags         ClassByTypeStats
	sources      ClassByTypeStats
	conventions  map[string]map[string]int
	holidayYears map[string]map[string]bool
}

func NewSearchStatsCacheImpl(mdb *sql.DB) SearchStatsCache {
	ssc := new(SearchStatsCacheImpl)
	ssc.mdb = mdb
	return ssc
}

func (ssc *SearchStatsCacheImpl) DoesHolidayExist(holiday string, year string) bool {
	return ssc.holidayYears[holiday][year]
}

func (ssc *SearchStatsCacheImpl) DoesConventionExist(location string, year string) bool {
	return ssc.conventions[year][location] > 0
}

func (ssc *SearchStatsCacheImpl) IsTagWithUnits(uid string, cts ...string) bool {
	return ssc.IsTagWithEnoughUnits(uid, 1, cts...)
}

func (ssc *SearchStatsCacheImpl) IsTagWithEnoughUnits(uid string, count int, cts ...string) bool {
	return ssc.isClassWithUnits("tags", uid, count, cts...)
}

func (ssc *SearchStatsCacheImpl) IsSourceWithUnits(uid string, cts ...string) bool {
	return ssc.IsSourceWithEnoughUnits(uid, 1, cts...)
}

func (ssc *SearchStatsCacheImpl) IsSourceWithEnoughUnits(uid string, count int, cts ...string) bool {
	return ssc.isClassWithUnits("sources", uid, count, cts...)
}

func (ssc *SearchStatsCacheImpl) isClassWithUnits(class, uid string, count int, cts ...string) bool {
	var stats ClassByTypeStats
	switch class {
	case "tags":
		stats = ssc.tags
	case "sources":
		stats = ssc.sources
	default:
		return false
	}

	if h, ok := stats[uid]; ok {
		for i := range cts {
			if c, ok := h[cts[i]]; ok && c >= count {
				return true
			}
		}
	}

	return false
}

func (ssc *SearchStatsCacheImpl) String() string {
	return "SearchStatsCacheImpl"
}

func (ssc *SearchStatsCacheImpl) Refresh() error {
	var err error
	ssc.tags, ssc.sources, err = ssc.load()
	if err != nil {
		return errors.Wrap(err, "Load tags and sources stats.")
	}
	ssc.conventions, err = ssc.refreshConventions()
	if err != nil {
		return errors.Wrap(err, "Load conventions stats.")
	}
	ssc.holidayYears, err = ssc.refreshHolidayYears()
	if err != nil {
		return errors.Wrap(err, "Load holidays stats.")
	}

	return nil
}

func (ssc *SearchStatsCacheImpl) refreshHolidayYears() (map[string]map[string]bool, error) {
	ret := make(map[string]map[string]bool)

	//  '1nyptSIo' is a const. uid for 'holidays' parent tag
	rows, err := queries.Raw(ssc.mdb, `select t.uid as tag_uid, 
		array_agg(distinct extract(year from (cu.properties ->> 'film_date')::date))
			from tags t join tags tp on t.parent_id = tp.id
		join content_units_tags cut on cut.tag_id = t.id
		join content_units cu on cu.id = cut.content_unit_id
			where tp.uid = '1nyptSIo' and cu.secure = 0 and cu.published = true
		group by t.uid`).Query()

	if err != nil {
		return nil, errors.Wrap(err, "refreshHolidays - Query failed.")
	}
	defer rows.Close()

	fmt.Printf("Init ret['']\n")
	ret[""] = make(map[string]bool) // Year without specific holiday
	fmt.Printf("ret[''] initialized\n")
	for rows.Next() {
		var tagUID string
		var years pq.StringArray
		err := rows.Scan(&tagUID, &years)
		if err != nil {
			return nil, errors.Wrap(err, "refreshHolidays rows.Scan")
		}
		fmt.Printf("Init ret[%s]\n", tagUID)
		ret[tagUID] = make(map[string]bool)
		fmt.Printf("ret[%s] initialized. Setting ret[%s][''] = true.\n", tagUID, tagUID)
		ret[tagUID][""] = true
		fmt.Printf("ret[%s][''] = true is set.\n", tagUID)
		for _, year := range years {
			fmt.Printf("Setting ret[''][%s] = true.\n", year)
			ret[""][year] = true
			fmt.Printf("ret[''][%s] = true is set. Setting ret[%s][%s] = true.\n", year, tagUID, year)
			ret[tagUID][year] = true
			fmt.Printf("ret[%s][%s] = true is set.\n", tagUID, year)
		}
	}

	return ret, nil
}

func (ssc *SearchStatsCacheImpl) refreshConventions() (map[string]map[string]int, error) {
	ret := make(map[string]map[string]int)
	var collections []*mdbmodels.Collection
	if err := mdbmodels.NewQuery(ssc.mdb,
		qm.From("collections as c"),
		qm.Where(fmt.Sprintf("c.type_id = %d", mdb.CONTENT_TYPE_REGISTRY.ByName[consts.CT_CONGRESS].ID))).
		Bind(&collections); err != nil {
		return nil, err
	}
	for _, c := range collections {
		// Accumulate convention by year and by location
		var props map[string]interface{}
		err := json.Unmarshal(c.Properties.JSON, &props)
		if err != nil {
			errors.Wrap(err, "Error reading collection convention properties.")
			continue
		}
		city := props["city"].(string)
		country := props["country"].(string)
		years := []string{""}
		if start_date := props["start_date"]; len(start_date.(string)) >= 4 {
			years = append(years, start_date.(string)[0:4])
		}
		if end_date := props["end_date"]; len(end_date.(string)) >= 4 && (len(years) == 0 || years[0] != end_date.(string)[0:4]) {
			years = append(years, end_date.(string)[0:4])
		}
		for _, year := range years {
			if _, ok := ret[year]; !ok {
				ret[year] = make(map[string]int)
			}
			ret[year][""]++ // Without location, just [congresses 2005]
			ret[year][city]++
			ret[year][country]++
			if city != "" && country != "" {
				ret[year][fmt.Sprintf("%s|%s", country, city)]++
			}
		}
	}
	return ret, nil
}

func (ssc *SearchStatsCacheImpl) load() (ClassByTypeStats, ClassByTypeStats, error) {
	rows, err := queries.Raw(ssc.mdb, `select
  t.id,
  t.parent_id,
  concat('t', t.uid),
  cu.type_id,
  count(cu.id)
from tags t
  left join content_units_tags cut on t.id = cut.tag_id
  left join (select * from content_units where content_units.secure = 0 and content_units.published is true) as cu on cut.content_unit_id = cu.id
group by t.id, cu.type_id
union
select
  s.id,
  s.parent_id,
  concat('s', s.uid),
  cu.type_id,
  count(cu.id)
from sources s
  left join content_units_sources cus on s.id = cus.source_id
  left join (select * from content_units where content_units.secure = 0 and content_units.published is true) as cu on cus.content_unit_id = cu.id
group by s.id, cu.type_id;`).Query()
	if err != nil {
		return nil, nil, errors.Wrap(err, "queries.Raw")
	}
	defer rows.Close()

	// TODO: take authors into consideration

	tags := NewStatsTree()
	sources := NewStatsTree()
	var tmp *StatsTree
	for rows.Next() {
		var k string
		var id int64
		var typeID, parentID null.Int64
		var count int
		err = rows.Scan(&id, &parentID, &k, &typeID, &count)
		if err != nil {
			return nil, nil, errors.Wrap(err, "rows.Scan")
		}

		if k[0] == 't' {
			tmp = tags
		} else if k[0] == 's' {
			tmp = sources
		}

		var ctName = ""
		if typeID.Valid {
			ct, ok := mdb.CONTENT_TYPE_REGISTRY.ByID[typeID.Int64]
			if ok {
				ctName = ct.Name
			} else {
				continue
			}
		}

		tmp.insert(id, parentID.Int64, k[1:], ctName, count)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, errors.Wrap(err, "rows.Err()")
	}

	tags.accumulate()
	sources.accumulate()

	return tags.flatten(), sources.flatten(), nil
}
