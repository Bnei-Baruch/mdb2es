package mdbmodels

import (
	"bytes"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/vattle/sqlboiler/boil"
	"github.com/vattle/sqlboiler/queries"
	"github.com/vattle/sqlboiler/queries/qm"
	"github.com/vattle/sqlboiler/strmangle"
	"gopkg.in/nullbio/null.v6"
)

// AuthorI18n is an object representing the database table.
type AuthorI18n struct {
	AuthorID  int64       `boil:"author_id" json:"author_id" toml:"author_id" yaml:"author_id"`
	Language  string      `boil:"language" json:"language" toml:"language" yaml:"language"`
	Name      null.String `boil:"name" json:"name,omitempty" toml:"name" yaml:"name,omitempty"`
	FullName  null.String `boil:"full_name" json:"full_name,omitempty" toml:"full_name" yaml:"full_name,omitempty"`
	CreatedAt time.Time   `boil:"created_at" json:"created_at" toml:"created_at" yaml:"created_at"`

	R *authorI18nR `boil:"-" json:"-" toml:"-" yaml:"-"`
	L authorI18nL  `boil:"-" json:"-" toml:"-" yaml:"-"`
}

// authorI18nR is where relationships are stored.
type authorI18nR struct {
	Author *Author
}

// authorI18nL is where Load methods for each relationship are stored.
type authorI18nL struct{}

var (
	authorI18nColumns               = []string{"author_id", "language", "name", "full_name", "created_at"}
	authorI18nColumnsWithoutDefault = []string{"author_id", "language", "name", "full_name"}
	authorI18nColumnsWithDefault    = []string{"created_at"}
	authorI18nPrimaryKeyColumns     = []string{"author_id", "language"}
)

type (
	// AuthorI18nSlice is an alias for a slice of pointers to AuthorI18n.
	// This should generally be used opposed to []AuthorI18n.
	AuthorI18nSlice []*AuthorI18n

	authorI18nQuery struct {
		*queries.Query
	}
)

// Cache for insert, update and upsert
var (
	authorI18nType                 = reflect.TypeOf(&AuthorI18n{})
	authorI18nMapping              = queries.MakeStructMapping(authorI18nType)
	authorI18nPrimaryKeyMapping, _ = queries.BindMapping(authorI18nType, authorI18nMapping, authorI18nPrimaryKeyColumns)
	authorI18nInsertCacheMut       sync.RWMutex
	authorI18nInsertCache          = make(map[string]insertCache)
	authorI18nUpdateCacheMut       sync.RWMutex
	authorI18nUpdateCache          = make(map[string]updateCache)
	authorI18nUpsertCacheMut       sync.RWMutex
	authorI18nUpsertCache          = make(map[string]insertCache)
)

var (
	// Force time package dependency for automated UpdatedAt/CreatedAt.
	_ = time.Second
	// Force bytes in case of primary key column that uses []byte (for relationship compares)
	_ = bytes.MinRead
)

// OneP returns a single authorI18n record from the query, and panics on error.
func (q authorI18nQuery) OneP() *AuthorI18n {
	o, err := q.One()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// One returns a single authorI18n record from the query.
func (q authorI18nQuery) One() (*AuthorI18n, error) {
	o := &AuthorI18n{}

	queries.SetLimit(q.Query, 1)

	err := q.Bind(o)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: failed to execute a one query for author_i18n")
	}

	return o, nil
}

// AllP returns all AuthorI18n records from the query, and panics on error.
func (q authorI18nQuery) AllP() AuthorI18nSlice {
	o, err := q.All()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// All returns all AuthorI18n records from the query.
func (q authorI18nQuery) All() (AuthorI18nSlice, error) {
	var o AuthorI18nSlice

	err := q.Bind(&o)
	if err != nil {
		return nil, errors.Wrap(err, "mdbmodels: failed to assign all query results to AuthorI18n slice")
	}

	return o, nil
}

// CountP returns the count of all AuthorI18n records in the query, and panics on error.
func (q authorI18nQuery) CountP() int64 {
	c, err := q.Count()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return c
}

// Count returns the count of all AuthorI18n records in the query.
func (q authorI18nQuery) Count() (int64, error) {
	var count int64

	queries.SetSelect(q.Query, nil)
	queries.SetCount(q.Query)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, "mdbmodels: failed to count author_i18n rows")
	}

	return count, nil
}

// Exists checks if the row exists in the table, and panics on error.
func (q authorI18nQuery) ExistsP() bool {
	e, err := q.Exists()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// Exists checks if the row exists in the table.
func (q authorI18nQuery) Exists() (bool, error) {
	var count int64

	queries.SetCount(q.Query)
	queries.SetLimit(q.Query, 1)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: failed to check if author_i18n exists")
	}

	return count > 0, nil
}

// AuthorG pointed to by the foreign key.
func (o *AuthorI18n) AuthorG(mods ...qm.QueryMod) authorQuery {
	return o.Author(boil.GetDB(), mods...)
}

// Author pointed to by the foreign key.
func (o *AuthorI18n) Author(exec boil.Executor, mods ...qm.QueryMod) authorQuery {
	queryMods := []qm.QueryMod{
		qm.Where("id=?", o.AuthorID),
	}

	queryMods = append(queryMods, mods...)

	query := Authors(exec, queryMods...)
	queries.SetFrom(query.Query, "\"authors\"")

	return query
}

// LoadAuthor allows an eager lookup of values, cached into the
// loaded structs of the objects.
func (authorI18nL) LoadAuthor(e boil.Executor, singular bool, maybeAuthorI18n interface{}) error {
	var slice []*AuthorI18n
	var object *AuthorI18n

	count := 1
	if singular {
		object = maybeAuthorI18n.(*AuthorI18n)
	} else {
		slice = *maybeAuthorI18n.(*AuthorI18nSlice)
		count = len(slice)
	}

	args := make([]interface{}, count)
	if singular {
		if object.R == nil {
			object.R = &authorI18nR{}
		}
		args[0] = object.AuthorID
	} else {
		for i, obj := range slice {
			if obj.R == nil {
				obj.R = &authorI18nR{}
			}
			args[i] = obj.AuthorID
		}
	}

	query := fmt.Sprintf(
		"select * from \"authors\" where \"id\" in (%s)",
		strmangle.Placeholders(dialect.IndexPlaceholders, count, 1, 1),
	)

	if boil.DebugMode {
		fmt.Fprintf(boil.DebugWriter, "%s\n%v\n", query, args)
	}

	results, err := e.Query(query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to eager load Author")
	}
	defer results.Close()

	var resultSlice []*Author
	if err = queries.Bind(results, &resultSlice); err != nil {
		return errors.Wrap(err, "failed to bind eager loaded slice Author")
	}

	if singular && len(resultSlice) != 0 {
		object.R.Author = resultSlice[0]
		return nil
	}

	for _, foreign := range resultSlice {
		for _, local := range slice {
			if local.AuthorID == foreign.ID {
				local.R.Author = foreign
				break
			}
		}
	}

	return nil
}

// SetAuthorG of the author_i18n to the related item.
// Sets o.R.Author to related.
// Adds o to related.R.AuthorI18ns.
// Uses the global database handle.
func (o *AuthorI18n) SetAuthorG(insert bool, related *Author) error {
	return o.SetAuthor(boil.GetDB(), insert, related)
}

// SetAuthorP of the author_i18n to the related item.
// Sets o.R.Author to related.
// Adds o to related.R.AuthorI18ns.
// Panics on error.
func (o *AuthorI18n) SetAuthorP(exec boil.Executor, insert bool, related *Author) {
	if err := o.SetAuthor(exec, insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetAuthorGP of the author_i18n to the related item.
// Sets o.R.Author to related.
// Adds o to related.R.AuthorI18ns.
// Uses the global database handle and panics on error.
func (o *AuthorI18n) SetAuthorGP(insert bool, related *Author) {
	if err := o.SetAuthor(boil.GetDB(), insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetAuthor of the author_i18n to the related item.
// Sets o.R.Author to related.
// Adds o to related.R.AuthorI18ns.
func (o *AuthorI18n) SetAuthor(exec boil.Executor, insert bool, related *Author) error {
	var err error
	if insert {
		if err = related.Insert(exec); err != nil {
			return errors.Wrap(err, "failed to insert into foreign table")
		}
	}

	updateQuery := fmt.Sprintf(
		"UPDATE \"author_i18n\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, []string{"author_id"}),
		strmangle.WhereClause("\"", "\"", 2, authorI18nPrimaryKeyColumns),
	)
	values := []interface{}{related.ID, o.AuthorID, o.Language}

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, updateQuery)
		fmt.Fprintln(boil.DebugWriter, values)
	}

	if _, err = exec.Exec(updateQuery, values...); err != nil {
		return errors.Wrap(err, "failed to update local table")
	}

	o.AuthorID = related.ID

	if o.R == nil {
		o.R = &authorI18nR{
			Author: related,
		}
	} else {
		o.R.Author = related
	}

	if related.R == nil {
		related.R = &authorR{
			AuthorI18ns: AuthorI18nSlice{o},
		}
	} else {
		related.R.AuthorI18ns = append(related.R.AuthorI18ns, o)
	}

	return nil
}

// AuthorI18nsG retrieves all records.
func AuthorI18nsG(mods ...qm.QueryMod) authorI18nQuery {
	return AuthorI18ns(boil.GetDB(), mods...)
}

// AuthorI18ns retrieves all the records using an executor.
func AuthorI18ns(exec boil.Executor, mods ...qm.QueryMod) authorI18nQuery {
	mods = append(mods, qm.From("\"author_i18n\""))
	return authorI18nQuery{NewQuery(exec, mods...)}
}

// FindAuthorI18nG retrieves a single record by ID.
func FindAuthorI18nG(authorID int64, language string, selectCols ...string) (*AuthorI18n, error) {
	return FindAuthorI18n(boil.GetDB(), authorID, language, selectCols...)
}

// FindAuthorI18nGP retrieves a single record by ID, and panics on error.
func FindAuthorI18nGP(authorID int64, language string, selectCols ...string) *AuthorI18n {
	retobj, err := FindAuthorI18n(boil.GetDB(), authorID, language, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// FindAuthorI18n retrieves a single record by ID with an executor.
// If selectCols is empty Find will return all columns.
func FindAuthorI18n(exec boil.Executor, authorID int64, language string, selectCols ...string) (*AuthorI18n, error) {
	authorI18nObj := &AuthorI18n{}

	sel := "*"
	if len(selectCols) > 0 {
		sel = strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, selectCols), ",")
	}
	query := fmt.Sprintf(
		"select %s from \"author_i18n\" where \"author_id\"=$1 AND \"language\"=$2", sel,
	)

	q := queries.Raw(exec, query, authorID, language)

	err := q.Bind(authorI18nObj)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: unable to select from author_i18n")
	}

	return authorI18nObj, nil
}

// FindAuthorI18nP retrieves a single record by ID with an executor, and panics on error.
func FindAuthorI18nP(exec boil.Executor, authorID int64, language string, selectCols ...string) *AuthorI18n {
	retobj, err := FindAuthorI18n(exec, authorID, language, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// InsertG a single record. See Insert for whitelist behavior description.
func (o *AuthorI18n) InsertG(whitelist ...string) error {
	return o.Insert(boil.GetDB(), whitelist...)
}

// InsertGP a single record, and panics on error. See Insert for whitelist
// behavior description.
func (o *AuthorI18n) InsertGP(whitelist ...string) {
	if err := o.Insert(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// InsertP a single record using an executor, and panics on error. See Insert
// for whitelist behavior description.
func (o *AuthorI18n) InsertP(exec boil.Executor, whitelist ...string) {
	if err := o.Insert(exec, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Insert a single record using an executor.
// Whitelist behavior: If a whitelist is provided, only those columns supplied are inserted
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns without a default value are included (i.e. name, age)
// - All columns with a default, but non-zero are included (i.e. health = 75)
func (o *AuthorI18n) Insert(exec boil.Executor, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no author_i18n provided for insertion")
	}

	var err error

	nzDefaults := queries.NonZeroDefaultSet(authorI18nColumnsWithDefault, o)

	key := makeCacheKey(whitelist, nzDefaults)
	authorI18nInsertCacheMut.RLock()
	cache, cached := authorI18nInsertCache[key]
	authorI18nInsertCacheMut.RUnlock()

	if !cached {
		wl, returnColumns := strmangle.InsertColumnSet(
			authorI18nColumns,
			authorI18nColumnsWithDefault,
			authorI18nColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)

		cache.valueMapping, err = queries.BindMapping(authorI18nType, authorI18nMapping, wl)
		if err != nil {
			return err
		}
		cache.retMapping, err = queries.BindMapping(authorI18nType, authorI18nMapping, returnColumns)
		if err != nil {
			return err
		}
		cache.query = fmt.Sprintf("INSERT INTO \"author_i18n\" (\"%s\") VALUES (%s)", strings.Join(wl, "\",\""), strmangle.Placeholders(dialect.IndexPlaceholders, len(wl), 1, 1))

		if len(cache.retMapping) != 0 {
			cache.query += fmt.Sprintf(" RETURNING \"%s\"", strings.Join(returnColumns, "\",\""))
		}
	}

	value := reflect.Indirect(reflect.ValueOf(o))
	vals := queries.ValuesFromMapping(value, cache.valueMapping)

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, cache.query)
		fmt.Fprintln(boil.DebugWriter, vals)
	}

	if len(cache.retMapping) != 0 {
		err = exec.QueryRow(cache.query, vals...).Scan(queries.PtrsFromMapping(value, cache.retMapping)...)
	} else {
		_, err = exec.Exec(cache.query, vals...)
	}

	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to insert into author_i18n")
	}

	if !cached {
		authorI18nInsertCacheMut.Lock()
		authorI18nInsertCache[key] = cache
		authorI18nInsertCacheMut.Unlock()
	}

	return nil
}

// UpdateG a single AuthorI18n record. See Update for
// whitelist behavior description.
func (o *AuthorI18n) UpdateG(whitelist ...string) error {
	return o.Update(boil.GetDB(), whitelist...)
}

// UpdateGP a single AuthorI18n record.
// UpdateGP takes a whitelist of column names that should be updated.
// Panics on error. See Update for whitelist behavior description.
func (o *AuthorI18n) UpdateGP(whitelist ...string) {
	if err := o.Update(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateP uses an executor to update the AuthorI18n, and panics on error.
// See Update for whitelist behavior description.
func (o *AuthorI18n) UpdateP(exec boil.Executor, whitelist ...string) {
	err := o.Update(exec, whitelist...)
	if err != nil {
		panic(boil.WrapErr(err))
	}
}

// Update uses an executor to update the AuthorI18n.
// Whitelist behavior: If a whitelist is provided, only the columns given are updated.
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns are inferred to start with
// - All primary keys are subtracted from this set
// Update does not automatically update the record in case of default values. Use .Reload()
// to refresh the records.
func (o *AuthorI18n) Update(exec boil.Executor, whitelist ...string) error {
	var err error
	key := makeCacheKey(whitelist, nil)
	authorI18nUpdateCacheMut.RLock()
	cache, cached := authorI18nUpdateCache[key]
	authorI18nUpdateCacheMut.RUnlock()

	if !cached {
		wl := strmangle.UpdateColumnSet(authorI18nColumns, authorI18nPrimaryKeyColumns, whitelist)
		if len(wl) == 0 {
			return errors.New("mdbmodels: unable to update author_i18n, could not build whitelist")
		}

		cache.query = fmt.Sprintf("UPDATE \"author_i18n\" SET %s WHERE %s",
			strmangle.SetParamNames("\"", "\"", 1, wl),
			strmangle.WhereClause("\"", "\"", len(wl)+1, authorI18nPrimaryKeyColumns),
		)
		cache.valueMapping, err = queries.BindMapping(authorI18nType, authorI18nMapping, append(wl, authorI18nPrimaryKeyColumns...))
		if err != nil {
			return err
		}
	}

	values := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), cache.valueMapping)

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, cache.query)
		fmt.Fprintln(boil.DebugWriter, values)
	}

	_, err = exec.Exec(cache.query, values...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update author_i18n row")
	}

	if !cached {
		authorI18nUpdateCacheMut.Lock()
		authorI18nUpdateCache[key] = cache
		authorI18nUpdateCacheMut.Unlock()
	}

	return nil
}

// UpdateAllP updates all rows with matching column names, and panics on error.
func (q authorI18nQuery) UpdateAllP(cols M) {
	if err := q.UpdateAll(cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values.
func (q authorI18nQuery) UpdateAll(cols M) error {
	queries.SetUpdate(q.Query, cols)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all for author_i18n")
	}

	return nil
}

// UpdateAllG updates all rows with the specified column values.
func (o AuthorI18nSlice) UpdateAllG(cols M) error {
	return o.UpdateAll(boil.GetDB(), cols)
}

// UpdateAllGP updates all rows with the specified column values, and panics on error.
func (o AuthorI18nSlice) UpdateAllGP(cols M) {
	if err := o.UpdateAll(boil.GetDB(), cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAllP updates all rows with the specified column values, and panics on error.
func (o AuthorI18nSlice) UpdateAllP(exec boil.Executor, cols M) {
	if err := o.UpdateAll(exec, cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values, using an executor.
func (o AuthorI18nSlice) UpdateAll(exec boil.Executor, cols M) error {
	ln := int64(len(o))
	if ln == 0 {
		return nil
	}

	if len(cols) == 0 {
		return errors.New("mdbmodels: update all requires at least one column argument")
	}

	colNames := make([]string, len(cols))
	args := make([]interface{}, len(cols))

	i := 0
	for name, value := range cols {
		colNames[i] = name
		args[i] = value
		i++
	}

	// Append all of the primary key values for each column
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), authorI18nPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf(
		"UPDATE \"author_i18n\" SET %s WHERE (\"author_id\",\"language\") IN (%s)",
		strmangle.SetParamNames("\"", "\"", 1, colNames),
		strmangle.Placeholders(dialect.IndexPlaceholders, len(o)*len(authorI18nPrimaryKeyColumns), len(colNames)+1, len(authorI18nPrimaryKeyColumns)),
	)

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all in authorI18n slice")
	}

	return nil
}

// UpsertG attempts an insert, and does an update or ignore on conflict.
func (o *AuthorI18n) UpsertG(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	return o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...)
}

// UpsertGP attempts an insert, and does an update or ignore on conflict. Panics on error.
func (o *AuthorI18n) UpsertGP(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpsertP attempts an insert using an executor, and does an update or ignore on conflict.
// UpsertP panics on error.
func (o *AuthorI18n) UpsertP(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(exec, updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Upsert attempts an insert using an executor, and does an update or ignore on conflict.
func (o *AuthorI18n) Upsert(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no author_i18n provided for upsert")
	}

	nzDefaults := queries.NonZeroDefaultSet(authorI18nColumnsWithDefault, o)

	// Build cache key in-line uglily - mysql vs postgres problems
	buf := strmangle.GetBuffer()
	if updateOnConflict {
		buf.WriteByte('t')
	} else {
		buf.WriteByte('f')
	}
	buf.WriteByte('.')
	for _, c := range conflictColumns {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	for _, c := range updateColumns {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	for _, c := range whitelist {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	for _, c := range nzDefaults {
		buf.WriteString(c)
	}
	key := buf.String()
	strmangle.PutBuffer(buf)

	authorI18nUpsertCacheMut.RLock()
	cache, cached := authorI18nUpsertCache[key]
	authorI18nUpsertCacheMut.RUnlock()

	var err error

	if !cached {
		var ret []string
		whitelist, ret = strmangle.InsertColumnSet(
			authorI18nColumns,
			authorI18nColumnsWithDefault,
			authorI18nColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)
		update := strmangle.UpdateColumnSet(
			authorI18nColumns,
			authorI18nPrimaryKeyColumns,
			updateColumns,
		)
		if len(update) == 0 {
			return errors.New("mdbmodels: unable to upsert author_i18n, could not build update column list")
		}

		conflict := conflictColumns
		if len(conflict) == 0 {
			conflict = make([]string, len(authorI18nPrimaryKeyColumns))
			copy(conflict, authorI18nPrimaryKeyColumns)
		}
		cache.query = queries.BuildUpsertQueryPostgres(dialect, "\"author_i18n\"", updateOnConflict, ret, update, conflict, whitelist)

		cache.valueMapping, err = queries.BindMapping(authorI18nType, authorI18nMapping, whitelist)
		if err != nil {
			return err
		}
		if len(ret) != 0 {
			cache.retMapping, err = queries.BindMapping(authorI18nType, authorI18nMapping, ret)
			if err != nil {
				return err
			}
		}
	}

	value := reflect.Indirect(reflect.ValueOf(o))
	vals := queries.ValuesFromMapping(value, cache.valueMapping)
	var returns []interface{}
	if len(cache.retMapping) != 0 {
		returns = queries.PtrsFromMapping(value, cache.retMapping)
	}

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, cache.query)
		fmt.Fprintln(boil.DebugWriter, vals)
	}

	if len(cache.retMapping) != 0 {
		err = exec.QueryRow(cache.query, vals...).Scan(returns...)
		if err == sql.ErrNoRows {
			err = nil // Postgres doesn't return anything when there's no update
		}
	} else {
		_, err = exec.Exec(cache.query, vals...)
	}
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to upsert author_i18n")
	}

	if !cached {
		authorI18nUpsertCacheMut.Lock()
		authorI18nUpsertCache[key] = cache
		authorI18nUpsertCacheMut.Unlock()
	}

	return nil
}

// DeleteP deletes a single AuthorI18n record with an executor.
// DeleteP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *AuthorI18n) DeleteP(exec boil.Executor) {
	if err := o.Delete(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteG deletes a single AuthorI18n record.
// DeleteG will match against the primary key column to find the record to delete.
func (o *AuthorI18n) DeleteG() error {
	if o == nil {
		return errors.New("mdbmodels: no AuthorI18n provided for deletion")
	}

	return o.Delete(boil.GetDB())
}

// DeleteGP deletes a single AuthorI18n record.
// DeleteGP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *AuthorI18n) DeleteGP() {
	if err := o.DeleteG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Delete deletes a single AuthorI18n record with an executor.
// Delete will match against the primary key column to find the record to delete.
func (o *AuthorI18n) Delete(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no AuthorI18n provided for delete")
	}

	args := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), authorI18nPrimaryKeyMapping)
	sql := "DELETE FROM \"author_i18n\" WHERE \"author_id\"=$1 AND \"language\"=$2"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete from author_i18n")
	}

	return nil
}

// DeleteAllP deletes all rows, and panics on error.
func (q authorI18nQuery) DeleteAllP() {
	if err := q.DeleteAll(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all matching rows.
func (q authorI18nQuery) DeleteAll() error {
	if q.Query == nil {
		return errors.New("mdbmodels: no authorI18nQuery provided for delete all")
	}

	queries.SetDelete(q.Query)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from author_i18n")
	}

	return nil
}

// DeleteAllGP deletes all rows in the slice, and panics on error.
func (o AuthorI18nSlice) DeleteAllGP() {
	if err := o.DeleteAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAllG deletes all rows in the slice.
func (o AuthorI18nSlice) DeleteAllG() error {
	if o == nil {
		return errors.New("mdbmodels: no AuthorI18n slice provided for delete all")
	}
	return o.DeleteAll(boil.GetDB())
}

// DeleteAllP deletes all rows in the slice, using an executor, and panics on error.
func (o AuthorI18nSlice) DeleteAllP(exec boil.Executor) {
	if err := o.DeleteAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all rows in the slice, using an executor.
func (o AuthorI18nSlice) DeleteAll(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no AuthorI18n slice provided for delete all")
	}

	if len(o) == 0 {
		return nil
	}

	var args []interface{}
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), authorI18nPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf(
		"DELETE FROM \"author_i18n\" WHERE (%s) IN (%s)",
		strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, authorI18nPrimaryKeyColumns), ","),
		strmangle.Placeholders(dialect.IndexPlaceholders, len(o)*len(authorI18nPrimaryKeyColumns), 1, len(authorI18nPrimaryKeyColumns)),
	)

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from authorI18n slice")
	}

	return nil
}

// ReloadGP refetches the object from the database and panics on error.
func (o *AuthorI18n) ReloadGP() {
	if err := o.ReloadG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadP refetches the object from the database with an executor. Panics on error.
func (o *AuthorI18n) ReloadP(exec boil.Executor) {
	if err := o.Reload(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadG refetches the object from the database using the primary keys.
func (o *AuthorI18n) ReloadG() error {
	if o == nil {
		return errors.New("mdbmodels: no AuthorI18n provided for reload")
	}

	return o.Reload(boil.GetDB())
}

// Reload refetches the object from the database
// using the primary keys with an executor.
func (o *AuthorI18n) Reload(exec boil.Executor) error {
	ret, err := FindAuthorI18n(exec, o.AuthorID, o.Language)
	if err != nil {
		return err
	}

	*o = *ret
	return nil
}

// ReloadAllGP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *AuthorI18nSlice) ReloadAllGP() {
	if err := o.ReloadAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *AuthorI18nSlice) ReloadAllP(exec boil.Executor) {
	if err := o.ReloadAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllG refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *AuthorI18nSlice) ReloadAllG() error {
	if o == nil {
		return errors.New("mdbmodels: empty AuthorI18nSlice provided for reload all")
	}

	return o.ReloadAll(boil.GetDB())
}

// ReloadAll refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *AuthorI18nSlice) ReloadAll(exec boil.Executor) error {
	if o == nil || len(*o) == 0 {
		return nil
	}

	authorI18ns := AuthorI18nSlice{}
	var args []interface{}
	for _, obj := range *o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), authorI18nPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf(
		"SELECT \"author_i18n\".* FROM \"author_i18n\" WHERE (%s) IN (%s)",
		strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, authorI18nPrimaryKeyColumns), ","),
		strmangle.Placeholders(dialect.IndexPlaceholders, len(*o)*len(authorI18nPrimaryKeyColumns), 1, len(authorI18nPrimaryKeyColumns)),
	)

	q := queries.Raw(exec, sql, args...)

	err := q.Bind(&authorI18ns)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to reload all in AuthorI18nSlice")
	}

	*o = authorI18ns

	return nil
}

// AuthorI18nExists checks if the AuthorI18n row exists.
func AuthorI18nExists(exec boil.Executor, authorID int64, language string) (bool, error) {
	var exists bool

	sql := "select exists(select 1 from \"author_i18n\" where \"author_id\"=$1 AND \"language\"=$2 limit 1)"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, authorID, language)
	}

	row := exec.QueryRow(sql, authorID, language)

	err := row.Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: unable to check if author_i18n exists")
	}

	return exists, nil
}

// AuthorI18nExistsG checks if the AuthorI18n row exists.
func AuthorI18nExistsG(authorID int64, language string) (bool, error) {
	return AuthorI18nExists(boil.GetDB(), authorID, language)
}

// AuthorI18nExistsGP checks if the AuthorI18n row exists. Panics on error.
func AuthorI18nExistsGP(authorID int64, language string) bool {
	e, err := AuthorI18nExists(boil.GetDB(), authorID, language)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// AuthorI18nExistsP checks if the AuthorI18n row exists. Panics on error.
func AuthorI18nExistsP(exec boil.Executor, authorID int64, language string) bool {
	e, err := AuthorI18nExists(exec, authorID, language)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}
