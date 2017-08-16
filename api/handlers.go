package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"github.com/vattle/sqlboiler/boil"
	"github.com/vattle/sqlboiler/queries"
	"github.com/vattle/sqlboiler/queries/qm"
	"gopkg.in/gin-gonic/gin.v1"
	"gopkg.in/olivere/elastic.v5"

	"github.com/Bnei-Baruch/archive-backend/mdb"
	"github.com/Bnei-Baruch/archive-backend/mdb/models"
	"github.com/Bnei-Baruch/archive-backend/utils"
)

var SECURE_PUBLISHED_MOD = qm.Where(fmt.Sprintf("secure=%d AND published IS TRUE", mdb.SEC_PUBLIC))

func CollectionsHandler(c *gin.Context) {
	var r CollectionsRequest
	if c.Bind(&r) != nil {
		return
	}

	resp, err := handleCollections(c.MustGet("MDB_DB").(*sql.DB), r)
	concludeRequest(c, resp, err)
}

func CollectionHandler(c *gin.Context) {
	var r ItemRequest
	if c.Bind(&r) != nil {
		return
	}

	r.UID = c.Param("uid")

	resp, err := handleCollection(c.MustGet("MDB_DB").(*sql.DB), r)
	concludeRequest(c, resp, err)
}

func ContentUnitsHandler(c *gin.Context) {
	var r ContentUnitsRequest
	if c.Bind(&r) != nil {
		return
	}

	resp, err := handleContentUnits(c.MustGet("MDB_DB").(*sql.DB), r)
	concludeRequest(c, resp, err)
}

func ContentUnitHandler(c *gin.Context) {
	var r BaseRequest
	if c.Bind(&r) != nil {
		return
	}

	db := c.MustGet("MDB_DB").(*sql.DB)

	uid := c.Param("uid")
	cu, err := mdbmodels.ContentUnits(db,
		SECURE_PUBLISHED_MOD,
		qm.Where("uid = ?", uid),
		qm.Load("Sources",
			"Tags",
			"CollectionsContentUnits",
			"CollectionsContentUnits.Collection",
			"DerivedContentUnitDerivations",
			"DerivedContentUnitDerivations.Source",
			"SourceContentUnitDerivations",
			"SourceContentUnitDerivations.Derived")).
		One()
	if err != nil {
		if err == sql.ErrNoRows {
			NewNotFoundError().Abort(c)
			return
		} else {
			NewInternalError(err).Abort(c)
			return
		}
	}

	u, err := mdbToCU(cu)
	if err != nil {
		NewInternalError(err).Abort(c)
		return
	}

	// Derived & Source content units
	cuidsMap := make(map[string]int64)

	u.SourceUnits = make(map[string]*ContentUnit)
	for _, cud := range cu.R.DerivedContentUnitDerivations {
		su := cud.R.Source
		if mdb.SEC_PUBLIC == su.Secure && su.Published {
			scu, err := mdbToCU(su)
			if err != nil {
				NewInternalError(err).Abort(c)
				return
			}

			// Dirty hack for unique mapping - needs to parse in client...
			key := fmt.Sprintf("%s____%s", su.UID, cud.Name)
			u.SourceUnits[key] = scu
			cuidsMap[key] = su.ID
		}
	}

	u.DerivedUnits = make(map[string]*ContentUnit)
	for _, cud := range cu.R.SourceContentUnitDerivations {
		du := cud.R.Derived
		if mdb.SEC_PUBLIC == du.Secure && du.Published {
			dcu, err := mdbToCU(du)
			if err != nil {
				NewInternalError(err).Abort(c)
				return
			}

			// Dirty hack for unique mapping - needs to parse in client...
			key := fmt.Sprintf("%s____%s", du.UID, cud.Name)
			u.DerivedUnits[key] = dcu
			cuidsMap[key] = du.ID
		}
	}

	cuids := make([]int64, 1)
	cuids[0] = cu.ID
	for _, v := range cuidsMap {
		cuids = append(cuids, v)
	}

	// content units i18n
	cui18nsMap, err := loadCUI18ns(db, r.Language, cuids)
	if err != nil {
		NewInternalError(err).Abort(c)
		return
	}
	if i18ns, ok := cui18nsMap[cu.ID]; ok {
		setCUI18n(u, r.Language, i18ns)
	}
	for k, v := range u.DerivedUnits {
		if i18ns, ok := cui18nsMap[cuidsMap[k]]; ok {
			setCUI18n(v, r.Language, i18ns)
		}
	}
	for k, v := range u.SourceUnits {
		if i18ns, ok := cui18nsMap[cuidsMap[k]]; ok {
			setCUI18n(v, r.Language, i18ns)
		}
	}

	// files (all CUs)
	fileMap, err := loadCUFiles(db, cuids)
	if err != nil {
		NewInternalError(err).Abort(c)
		return
	}
	if files, ok := fileMap[cu.ID]; ok {
		if err := setCUFiles(u, files); err != nil {
			NewInternalError(err).Abort(c)
			return
		}
	}
	for k, v := range u.DerivedUnits {
		if files, ok := fileMap[cuidsMap[k]]; ok {
			if err := setCUFiles(v, files); err != nil {
				NewInternalError(err).Abort(c)
				return
			}
		}
	}
	for k, v := range u.SourceUnits {
		if files, ok := fileMap[cuidsMap[k]]; ok {
			if err := setCUFiles(v, files); err != nil {
				NewInternalError(err).Abort(c)
				return
			}
		}
	}

	// collections
	u.Collections = make(map[string]*Collection)
	cidsMap := make(map[string]int64)
	for _, ccu := range cu.R.CollectionsContentUnits {
		if mdb.SEC_PUBLIC == ccu.R.Collection.Secure && ccu.R.Collection.Published {
			cl := ccu.R.Collection

			cc, err := mdbToC(cl)
			if err != nil {
				NewInternalError(err).Abort(c)
				return
			}

			// Dirty hack for unique mapping - needs to parse in client...
			key := fmt.Sprintf("%s____%s", cl.UID, ccu.Name)
			u.Collections[key] = cc

			cidsMap[key] = cl.ID
		}
	}

	// collections - i18n
	cids := make([]int64, 0)
	for _, v := range cidsMap {
		cids = append(cids, v)
	}
	if len(cids) > 0 {
		ci18nsMap, err := loadCI18ns(db, r.Language, cids)
		if err != nil {
			NewInternalError(err).Abort(c)
			return
		}
		for k, v := range u.Collections {
			if i18ns, ok := ci18nsMap[cidsMap[k]]; ok {
				setCI18n(v, r.Language, i18ns)
			}
		}
	}

	// sources
	u.Sources = make([]string, len(cu.R.Sources))
	for i, x := range cu.R.Sources {
		u.Sources[i] = x.UID
	}

	// tags
	u.Tags = make([]string, len(cu.R.Tags))
	for i, x := range cu.R.Tags {
		u.Tags[i] = x.UID
	}

	c.JSON(http.StatusOK, u)
}

func LessonsHandler(c *gin.Context) {
	var r LessonsRequest
	if c.Bind(&r) != nil {
		return
	}

	// We're either in full lessons mode or lesson parts mode based on
	// filters that apply only to lesson parts (content_units)

	if utils.IsEmpty(r.Authors) &&
		utils.IsEmpty(r.Sources) &&
		utils.IsEmpty(r.Tags) {
		if r.OrderBy == "" {
			r.OrderBy = "(properties->>'film_date')::date desc, (properties->>'number')::int desc, created_at desc"
		}
		cr := CollectionsRequest{
			ContentTypesFilter: ContentTypesFilter{
				ContentTypes: []string{mdb.CT_DAILY_LESSON, mdb.CT_SPECIAL_LESSON},
			},
			ListRequest:     r.ListRequest,
			DateRangeFilter: r.DateRangeFilter,
		}
		resp, err := handleCollections(c.MustGet("MDB_DB").(*sql.DB), cr)
		concludeRequest(c, resp, err)
	} else {
		if r.OrderBy == "" {
			r.OrderBy = "(properties->>'film_date')::date desc, created_at desc"
		}
		cur := ContentUnitsRequest{
			ContentTypesFilter: ContentTypesFilter{
				ContentTypes: []string{mdb.CT_LESSON_PART},
			},
			ListRequest:     r.ListRequest,
			DateRangeFilter: r.DateRangeFilter,
			SourcesFilter:   r.SourcesFilter,
			TagsFilter:      r.TagsFilter,
		}
		resp, err := handleContentUnits(c.MustGet("MDB_DB").(*sql.DB), cur)
		concludeRequest(c, resp, err)
	}
}

func SearchHandler(c *gin.Context) {
	text := c.Query("text")
	if text == "" {
		NewBadRequestError(errors.New("Can't search for an empty text")).Abort(c)
		return
	}

	page := 0
	pageQ := c.Query("page")
	if pageQ != "" {
		var err error
		page, err = strconv.Atoi(pageQ)
		if err != nil {
			NewBadRequestError(err).Abort(c)
			return
		}
	}

	res, err := handleSearch(c.MustGet("ES_CLIENT").(*elastic.Client), "mdb_collections", text, page)
	if err == nil {
		c.JSON(http.StatusOK, res)
	} else {
		NewInternalError(err).Abort(c)
	}
}

func handleCollections(db *sql.DB, r CollectionsRequest) (*CollectionsResponse, *HttpError) {
	mods := []qm.QueryMod{SECURE_PUBLISHED_MOD}

	// filters
	if err := appendContentTypesFilterMods(&mods, r.ContentTypesFilter); err != nil {
		return nil, NewBadRequestError(err)
	}
	if err := appendDateRangeFilterMods(&mods, r.DateRangeFilter); err != nil {
		return nil, NewBadRequestError(err)
	}

	// count query
	total, err := mdbmodels.Collections(db, mods...).Count()
	if err != nil {
		return nil, NewInternalError(err)
	}
	if total == 0 {
		return NewCollectionsResponse(), nil
	}

	// order, limit, offset
	_, offset, err := appendListMods(&mods, r.ListRequest)
	if err != nil {
		return nil, NewBadRequestError(err)
	}
	if int64(offset) >= total {
		return NewCollectionsResponse(), nil
	}

	// Eager loading
	mods = append(mods, qm.Load(
		"CollectionsContentUnits",
		"CollectionsContentUnits.ContentUnit"))

	// data query
	collections, err := mdbmodels.Collections(db, mods...).All()
	if err != nil {
		return nil, NewInternalError(err)
	}

	// Filter secure & published content units
	// Load i18n for all collections and all units - total 2 DB round trips
	cids := make([]int64, len(collections))
	cuids := make([]int64, 0)
	for i, x := range collections {
		cids[i] = x.ID
		b := x.R.CollectionsContentUnits[:0]
		for _, y := range x.R.CollectionsContentUnits {

			// Edo: Commenting out as I can't reproduce
			// Workaround for this bug: https://github.com/vattle/sqlboiler/issues/154
			//if y.R.ContentUnit == nil {
			//	err = y.L.LoadContentUnit(db, true, y)
			//	if err != nil {
			//		return nil, NewInternalError(err)
			//	}
			//}

			if mdb.SEC_PUBLIC == y.R.ContentUnit.Secure && y.R.ContentUnit.Published {
				b = append(b, y)
				cuids = append(cuids, y.ContentUnitID)
			}
			x.R.CollectionsContentUnits = b
		}
	}

	ci18nsMap, err := loadCI18ns(db, r.Language, cids)
	if err != nil {
		return nil, NewInternalError(err)
	}
	cui18nsMap, err := loadCUI18ns(db, r.Language, cuids)
	if err != nil {
		return nil, NewInternalError(err)
	}

	// Response
	resp := &CollectionsResponse{
		ListResponse: ListResponse{Total: total},
		Collections:  make([]*Collection, len(collections)),
	}
	for i, x := range collections {
		c, err := mdbToC(x)
		if err != nil {
			return nil, NewInternalError(err)
		}
		if i18ns, ok := ci18nsMap[x.ID]; ok {
			setCI18n(c, r.Language, i18ns)
		}

		// content units
		sort.SliceStable(x.R.CollectionsContentUnits, func (i int, j int) bool {
			return x.R.CollectionsContentUnits[i].Position < x.R.CollectionsContentUnits[j].Position
		})
		//sort.Sort(mdb.InCollection{ExtCCUSlice: mdb.ExtCCUSlice(x.R.CollectionsContentUnits)})
		c.ContentUnits = make([]*ContentUnit, 0)
		for _, ccu := range x.R.CollectionsContentUnits {
			cu := ccu.R.ContentUnit

			u, err := mdbToCU(cu)
			if err != nil {
				return nil, NewInternalError(err)
			}
			if i18ns, ok := cui18nsMap[cu.ID]; ok {
				setCUI18n(u, r.Language, i18ns)
			}

			u.NameInCollection = ccu.Name
			c.ContentUnits = append(c.ContentUnits, u)
		}
		resp.Collections[i] = c
	}

	return resp, nil
}

func handleCollection(db *sql.DB, r ItemRequest) (*Collection, *HttpError) {

	c, err := mdbmodels.Collections(db,
		SECURE_PUBLISHED_MOD,
		qm.Where("uid = ?", r.UID),
		qm.Load("CollectionsContentUnits",
			"CollectionsContentUnits.ContentUnit")).
		One()
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, NewNotFoundError()
		} else {
			return nil, NewInternalError(err)
		}
	}

	// collection
	cl, err := mdbToC(c)
	if err != nil {
		return nil, NewInternalError(err)
	}

	// collection i18n
	ci18nsMap, err := loadCI18ns(db, r.Language, []int64{c.ID})
	if err != nil {
		return nil, NewInternalError(err)
	}
	if i18ns, ok := ci18nsMap[c.ID]; ok {
		setCI18n(cl, r.Language, i18ns)
	}

	// content units
	cuids := make([]int64, 0)

	// filter secure & published
	b := c.R.CollectionsContentUnits[:0]
	for _, y := range c.R.CollectionsContentUnits {
		if mdb.SEC_PUBLIC == y.R.ContentUnit.Secure && y.R.ContentUnit.Published {
			b = append(b, y)
			cuids = append(cuids, y.ContentUnitID)
		}
		c.R.CollectionsContentUnits = b
	}

	// load i18ns
	cui18nsMap, err := loadCUI18ns(db, r.Language, cuids)
	if err != nil {
		return nil, NewInternalError(err)
	}

	// sort by ccu.name
	sort.SliceStable(c.R.CollectionsContentUnits, func (i int, j int) bool {
		return c.R.CollectionsContentUnits[i].Position < c.R.CollectionsContentUnits[j].Position
	})
	//sort.Sort(mdb.InCollection{ExtCCUSlice: mdb.ExtCCUSlice(c.R.CollectionsContentUnits)})

	// construct DTO's
	cl.ContentUnits = make([]*ContentUnit, 0)
	for _, ccu := range c.R.CollectionsContentUnits {
		cu := ccu.R.ContentUnit

		u, err := mdbToCU(cu)
		if err != nil {
			return nil, NewInternalError(err)
		}
		if i18ns, ok := cui18nsMap[cu.ID]; ok {
			setCUI18n(u, r.Language, i18ns)
		}

		u.NameInCollection = ccu.Name
		cl.ContentUnits = append(cl.ContentUnits, u)
	}

	return cl, nil
}

func handleContentUnits(db *sql.DB, r ContentUnitsRequest) (*ContentUnitsResponse, *HttpError) {
	mods := []qm.QueryMod{SECURE_PUBLISHED_MOD}

	// filters
	if err := appendContentTypesFilterMods(&mods, r.ContentTypesFilter); err != nil {
		return nil, NewBadRequestError(err)
	}
	if err := appendDateRangeFilterMods(&mods, r.DateRangeFilter); err != nil {
		return nil, NewBadRequestError(err)
	}
	if err := appendSourcesFilterMods(db, &mods, r.SourcesFilter); err != nil {
		if e, ok := err.(*HttpError); ok {
			return nil, e
		} else {
			return nil, NewInternalError(err)
		}
	}
	if err := appendTagsFilterMods(db, &mods, r.TagsFilter); err != nil {
		return nil, NewInternalError(err)
	}

	// count query
	total, err := mdbmodels.ContentUnits(db, mods...).Count()
	if err != nil {
		return nil, NewInternalError(err)
	}
	if total == 0 {
		return NewContentUnitsResponse(), nil
	}

	// order, limit, offset
	_, offset, err := appendListMods(&mods, r.ListRequest)
	if err != nil {
		return nil, NewBadRequestError(err)
	}
	if int64(offset) >= total {
		return NewContentUnitsResponse(), nil
	}

	// Eager loading
	mods = append(mods, qm.Load(
		"CollectionsContentUnits",
		"CollectionsContentUnits.Collection"))

	// data query
	units, err := mdbmodels.ContentUnits(db, mods...).All()
	if err != nil {
		return nil, NewInternalError(err)
	}

	// Filter secure published collections
	// Load i18n for all content units and all collections - total 2 DB round trips
	cuids := make([]int64, len(units))
	cids := make([]int64, 0)
	for i, x := range units {
		cuids[i] = x.ID
		b := x.R.CollectionsContentUnits[:0]
		for _, y := range x.R.CollectionsContentUnits {

			// Edo: Commenting out as I can't reproduce
			// Workaround for this bug: https://github.com/vattle/sqlboiler/issues/154
			//if y.R.Collection == nil {
			//	err = y.L.LoadCollection(db, true, y)
			//	if err != nil {
			//		return nil, NewInternalError(err)
			//	}
			//}

			if mdb.SEC_PUBLIC == y.R.Collection.Secure && y.R.Collection.Published {
				b = append(b, y)
				cids = append(cids, y.CollectionID)
			}
			x.R.CollectionsContentUnits = b
		}
	}

	cui18nsMap, err := loadCUI18ns(db, r.Language, cuids)
	if err != nil {
		return nil, NewInternalError(err)
	}
	ci18nsMap, err := loadCI18ns(db, r.Language, cids)
	if err != nil {
		return nil, NewInternalError(err)
	}

	// Response
	resp := &ContentUnitsResponse{
		ListResponse: ListResponse{Total: total},
		ContentUnits: make([]*ContentUnit, len(units)),
	}
	for i, x := range units {
		cu, err := mdbToCU(x)
		if err != nil {
			return nil, NewInternalError(err)
		}
		if i18ns, ok := cui18nsMap[x.ID]; ok {
			setCUI18n(cu, r.Language, i18ns)
		}

		// collections
		cu.Collections = make(map[string]*Collection, 0)
		for _, ccu := range x.R.CollectionsContentUnits {
			cl := ccu.R.Collection

			cc, err := mdbToC(cl)
			if err != nil {
				return nil, NewInternalError(err)
			}
			if i18ns, ok := ci18nsMap[cl.ID]; ok {
				setCI18n(cc, r.Language, i18ns)
			}

			// Dirty hack for unique mapping - needs to parse in client...
			key := fmt.Sprintf("%s____%s", cl.UID, ccu.Name)
			cu.Collections[key] = cc
		}
		resp.ContentUnits[i] = cu
	}

	return resp, nil
}

func handleSearch(esc *elastic.Client, index string, text string, from int) (*elastic.SearchResult, error) {
	q := elastic.NewNestedQuery("content_units",
		elastic.NewMultiMatchQuery(text, "content_units.names.*", "content_units.descriptions.*"))

	h := elastic.NewHighlight().HighlighQuery(q)

	return esc.Search().
		Index(index).
		Query(q).
		Highlight(h).
		From(from).
		Do(context.TODO())
}

// appendListMods compute and appends the OrderBy, Limit and Offset query mods.
// It returns the limit, offset and error if any
func appendListMods(mods *[]qm.QueryMod, r ListRequest) (int, int, error) {
	if r.OrderBy == "" {
		*mods = append(*mods,
			qm.OrderBy("(coalesce(properties->>'film_date', properties->>'start_date', created_at::text))::date desc, created_at desc"))
	} else {
		*mods = append(*mods, qm.OrderBy(r.OrderBy))
	}

	var limit, offset int

	if r.StartIndex == 0 {
		// pagination style
		if r.PageSize == 0 {
			limit = DEFAULT_PAGE_SIZE
		} else {
			limit = utils.Min(r.PageSize, MAX_PAGE_SIZE)
		}
		if r.PageNumber > 1 {
			offset = (r.PageNumber - 1) * limit
		}
	} else {
		// start & stop index style for "infinite" lists
		offset = r.StartIndex - 1
		if r.StopIndex == 0 {
			limit = MAX_PAGE_SIZE
		} else if r.StopIndex < r.StartIndex {
			return 0, 0, errors.Errorf("Invalid range [%d-%d]", r.StartIndex, r.StopIndex)
		} else {
			limit = r.StopIndex - r.StartIndex + 1
		}
	}

	*mods = append(*mods, qm.Limit(limit))
	if offset != 0 {
		*mods = append(*mods, qm.Offset(offset))
	}

	return limit, offset, nil
}

func appendContentTypesFilterMods(mods *[]qm.QueryMod, f ContentTypesFilter) error {
	if utils.IsEmpty(f.ContentTypes) {
		return nil
	}

	a := make([]interface{}, len(f.ContentTypes))
	for i, x := range f.ContentTypes {
		ct, ok := mdb.CONTENT_TYPE_REGISTRY.ByName[strings.ToUpper(x)]
		if ok {
			a[i] = ct.ID
		} else {
			return errors.Errorf("Unknown content type: %s", x)
		}
	}

	*mods = append(*mods, qm.WhereIn("type_id IN ?", a...))

	return nil
}

func appendDateRangeFilterMods(mods *[]qm.QueryMod, f DateRangeFilter) error {
	s, e, err := f.Range()
	if err != nil {
		return err
	}

	if f.StartDate != "" && f.EndDate != "" && e.Before(s) {
		return errors.New("Invalid date range")
	}

	if f.StartDate != "" {
		*mods = append(*mods, qm.Where("(properties->>'film_date')::date >= ?", s))
	}
	if f.EndDate != "" {
		*mods = append(*mods, qm.Where("(properties->>'film_date')::date <= ?", e))
	}

	return nil
}

func appendSourcesFilterMods(exec boil.Executor, mods *[]qm.QueryMod, f SourcesFilter) error {
	if utils.IsEmpty(f.Authors) && len(f.Sources) == 0 {
		return nil
	}

	// slice of all source ids we want
	source_uids := make([]string, 0)

	// fetch source ids by authors
	if !utils.IsEmpty(f.Authors) {
		for _, x := range f.Authors {
			if _, ok := mdb.AUTHOR_REGISTRY.ByCode[strings.ToLower(x)]; !ok {
				return NewBadRequestError(errors.Errorf("Unknown author: %s", x))
			}
		}

		var uids pq.StringArray
		q := `SELECT array_agg(DISTINCT s.uid)
		      FROM authors a INNER JOIN authors_sources "as" ON a.id = "as".author_id
		      INNER JOIN sources s ON "as".source_id = s.id
		      WHERE a.code = ANY($1)`
		err := queries.Raw(exec, q, pq.Array(f.Authors)).QueryRow().Scan(&uids)
		if err != nil {
			return err
		}
		source_uids = append(source_uids, uids...)
	}

	// blend in requested sources
	source_uids = append(source_uids, f.Sources...)

	// find all nested source_uids
	q := `WITH RECURSIVE rec_sources AS (
		  SELECT s.id FROM sources s WHERE s.uid = ANY($1)
		  UNION
		  SELECT s.id FROM sources s INNER JOIN rec_sources rs ON s.parent_id = rs.id
	      )
	      SELECT array_agg(distinct id) FROM rec_sources`
	var ids pq.Int64Array
	err := queries.Raw(exec, q, pq.Array(source_uids)).QueryRow().Scan(&ids)
	if err != nil {
		return err
	}

	if ids == nil || len(ids) == 0 {
		*mods = append(*mods, qm.Where("id < 0")) // so results would be empty
	} else {
		*mods = append(*mods,
			qm.InnerJoin("content_units_sources cus ON id = cus.content_unit_id"),
			qm.WhereIn("cus.source_id in ?", utils.ConvertArgsInt64(ids)...))
	}

	return nil
}

func appendTagsFilterMods(exec boil.Executor, mods *[]qm.QueryMod, f TagsFilter) error {
	if len(f.Tags) == 0 {
		return nil
	}

	// find all nested tag_ids
	q := `WITH RECURSIVE rec_tags AS (
	        SELECT t.id FROM tags t WHERE t.uid = ANY($1)
	        UNION
	        SELECT t.id FROM tags t INNER JOIN rec_tags rt ON t.parent_id = rt.id
	      )
	      SELECT array_agg(distinct id) FROM rec_tags`
	var ids pq.Int64Array
	err := queries.Raw(exec, q, pq.Array(f.Tags)).QueryRow().Scan(&ids)
	if err != nil {
		return err
	}

	if ids == nil || len(ids) == 0 {
		*mods = append(*mods, qm.Where("id < 0")) // so results would be empty
	} else {
		*mods = append(*mods,
			qm.InnerJoin("content_units_tags cut ON id = cut.content_unit_id"),
			qm.WhereIn("cut.tag_id in ?", utils.ConvertArgsInt64(ids)...))
	}

	return nil
}

// concludeRequest responds with JSON of given response or aborts the request with the given error.
func concludeRequest(c *gin.Context, resp interface{}, err *HttpError) {
	if err == nil {
		c.JSON(http.StatusOK, resp)
	} else {
		err.Abort(c)
	}
}

func mdbToC(c *mdbmodels.Collection) (cl *Collection, err error) {
	var props mdb.CollectionProperties
	if err = c.Properties.Unmarshal(&props); err != nil {
		err = errors.Wrap(err, "json.Unmarshal properties")
		return
	}

	cl = &Collection{
		ID:          c.UID,
		ContentType: mdb.CONTENT_TYPE_REGISTRY.ByID[c.TypeID].Name,
		Country:     props.Country,
		City:        props.City,
		FullAddress: props.FullAddress,
	}

	if !props.FilmDate.IsZero() {
		cl.FilmDate = &Date{Time: props.FilmDate.Time}
	}
	if !props.StartDate.IsZero() {
		cl.StartDate = &Date{Time: props.StartDate.Time}
	}
	if !props.EndDate.IsZero() {
		cl.EndDate = &Date{Time: props.EndDate.Time}
	}

	return
}

func mdbToCU(cu *mdbmodels.ContentUnit) (*ContentUnit, error) {
	var props mdb.ContentUnitProperties
	if err := cu.Properties.Unmarshal(&props); err != nil {
		return nil, errors.Wrap(err, "json.Unmarshal properties")
	}

	u := &ContentUnit{
		ID:               cu.UID,
		ContentType:      mdb.CONTENT_TYPE_REGISTRY.ByID[cu.TypeID].Name,
		Duration:         props.Duration,
		OriginalLanguage: props.OriginalLanguage,
	}

	if !props.FilmDate.IsZero() {
		u.FilmDate = &Date{Time: props.FilmDate.Time}
	}

	return u, nil
}

func mdbToFile(file *mdbmodels.File) (*File, error) {
	var props mdb.FileProperties
	if err := file.Properties.Unmarshal(&props); err != nil {
		return nil, errors.Wrap(err, "json.Unmarshal properties")
	}

	f := &File{
		ID:       file.UID,
		Name:     file.Name,
		Size:     file.Size,
		Type:     file.Type,
		SubType:  file.SubType,
		Duration: props.Duration,
	}

	if file.Language.Valid {
		f.Language = file.Language.String
	}
	if file.MimeType.Valid {
		f.MimeType = file.MimeType.String
	}

	return f, nil
}

func loadCI18ns(db *sql.DB, language string, ids []int64) (map[int64]map[string]*mdbmodels.CollectionI18n, error) {
	// Load from DB
	i18ns, err := mdbmodels.CollectionI18ns(db,
		qm.WhereIn("collection_id in ?", utils.ConvertArgsInt64(ids)...),
		qm.AndIn("language in ?", utils.ConvertArgsString(LANG_ORDER[language])...)).
		All()
	if err != nil {
		return nil, errors.Wrap(err, "Load collections i18ns from DB")
	}

	// Group by collection and language
	i18nsMap := make(map[int64]map[string]*mdbmodels.CollectionI18n, len(ids))
	for _, x := range i18ns {
		v, ok := i18nsMap[x.CollectionID]
		if !ok {
			v = make(map[string]*mdbmodels.CollectionI18n, 1)
			i18nsMap[x.CollectionID] = v
		}
		v[x.Language] = x
	}

	return i18nsMap, nil
}

func setCI18n(c *Collection, language string, i18ns map[string]*mdbmodels.CollectionI18n) {
	for _, l := range LANG_ORDER[language] {
		li18n, ok := i18ns[l]
		if ok {
			if c.Name == "" && li18n.Name.Valid {
				c.Name = li18n.Name.String
			}
			if c.Description == "" && li18n.Description.Valid {
				c.Description = li18n.Description.String
			}
		}
	}
}

func loadCUI18ns(db *sql.DB, language string, ids []int64) (map[int64]map[string]*mdbmodels.ContentUnitI18n, error) {
	// Load from DB
	i18ns, err := mdbmodels.ContentUnitI18ns(db,
		qm.WhereIn("content_unit_id in ?", utils.ConvertArgsInt64(ids)...),
		qm.AndIn("language in ?", utils.ConvertArgsString(LANG_ORDER[language])...)).
		All()
	if err != nil {
		return nil, errors.Wrap(err, "Load content units i18ns from DB")
	}

	// Group by content unit and language
	i18nsMap := make(map[int64]map[string]*mdbmodels.ContentUnitI18n, len(ids))
	for _, x := range i18ns {
		v, ok := i18nsMap[x.ContentUnitID]
		if !ok {
			v = make(map[string]*mdbmodels.ContentUnitI18n, 1)
			i18nsMap[x.ContentUnitID] = v
		}
		v[x.Language] = x
	}

	return i18nsMap, nil
}

func loadCUFiles(db *sql.DB, ids []int64) (map[int64][]*mdbmodels.File, error) {
	// Load from DB
	allFiles, err := mdbmodels.Files(db,
		SECURE_PUBLISHED_MOD,
		qm.WhereIn("content_unit_id in ?", utils.ConvertArgsInt64(ids)...)).
		All()
	if err != nil {
		return nil, errors.Wrap(err, "Load files from DB")
	}

	// Group by content unit
	filesMap := make(map[int64][]*mdbmodels.File, len(ids))
	for _, x := range allFiles {
		v, ok := filesMap[x.ContentUnitID.Int64]
		if ok {
			v = append(v, x)
		} else {
			v = []*mdbmodels.File{x}
		}
		filesMap[x.ContentUnitID.Int64] = v
	}

	return filesMap, nil
}

func setCUI18n(cu *ContentUnit, language string, i18ns map[string]*mdbmodels.ContentUnitI18n) {
	for _, l := range LANG_ORDER[language] {
		li18n, ok := i18ns[l]
		if ok {
			if cu.Name == "" && li18n.Name.Valid {
				cu.Name = li18n.Name.String
			}
			if cu.Description == "" && li18n.Description.Valid {
				cu.Description = li18n.Description.String
			}
		}
	}
}

func setCUFiles(cu *ContentUnit, files []*mdbmodels.File) error {
	cu.Files = make([]*File, len(files))

	for i, x := range files {
		f, err := mdbToFile(x)
		if err != nil {
			return err
		}
		cu.Files[i] = f
	}

	return nil
}
