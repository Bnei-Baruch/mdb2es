// Code generated by SQLBoiler (https://github.com/Bnei-Baruch/sqlboiler). DO NOT EDIT.
// This file is meant to be re-generated in place and/or deleted at any time.

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
	"github.com/Bnei-Baruch/sqlboiler/boil"
	"github.com/Bnei-Baruch/sqlboiler/queries"
	"github.com/Bnei-Baruch/sqlboiler/queries/qm"
	"github.com/Bnei-Baruch/sqlboiler/strmangle"
)

// Storage is an object representing the database table.
type Storage struct {
	ID       int64  `boil:"id" json:"id" toml:"id" yaml:"id"`
	Name     string `boil:"name" json:"name" toml:"name" yaml:"name"`
	Country  string `boil:"country" json:"country" toml:"country" yaml:"country"`
	Location string `boil:"location" json:"location" toml:"location" yaml:"location"`
	Status   string `boil:"status" json:"status" toml:"status" yaml:"status"`
	Access   string `boil:"access" json:"access" toml:"access" yaml:"access"`

	R *storageR `boil:"-" json:"-" toml:"-" yaml:"-"`
	L storageL  `boil:"-" json:"-" toml:"-" yaml:"-"`
}

var StorageColumns = struct {
	ID       string
	Name     string
	Country  string
	Location string
	Status   string
	Access   string
}{
	ID:       "id",
	Name:     "name",
	Country:  "country",
	Location: "location",
	Status:   "status",
	Access:   "access",
}

// storageR is where relationships are stored.
type storageR struct {
	Files FileSlice
}

// storageL is where Load methods for each relationship are stored.
type storageL struct{}

var (
	storageColumns               = []string{"id", "name", "country", "location", "status", "access"}
	storageColumnsWithoutDefault = []string{"name", "country", "location", "status", "access"}
	storageColumnsWithDefault    = []string{"id"}
	storagePrimaryKeyColumns     = []string{"id"}
)

type (
	// StorageSlice is an alias for a slice of pointers to Storage.
	// This should generally be used opposed to []Storage.
	StorageSlice []*Storage

	storageQuery struct {
		*queries.Query
	}
)

// Cache for insert, update and upsert
var (
	storageType                 = reflect.TypeOf(&Storage{})
	storageMapping              = queries.MakeStructMapping(storageType)
	storagePrimaryKeyMapping, _ = queries.BindMapping(storageType, storageMapping, storagePrimaryKeyColumns)
	storageInsertCacheMut       sync.RWMutex
	storageInsertCache          = make(map[string]insertCache)
	storageUpdateCacheMut       sync.RWMutex
	storageUpdateCache          = make(map[string]updateCache)
	storageUpsertCacheMut       sync.RWMutex
	storageUpsertCache          = make(map[string]insertCache)
)

var (
	// Force time package dependency for automated UpdatedAt/CreatedAt.
	_ = time.Second
	// Force bytes in case of primary key column that uses []byte (for relationship compares)
	_ = bytes.MinRead
)

// OneP returns a single storage record from the query, and panics on error.
func (q storageQuery) OneP() *Storage {
	o, err := q.One()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// One returns a single storage record from the query.
func (q storageQuery) One() (*Storage, error) {
	o := &Storage{}

	queries.SetLimit(q.Query, 1)

	err := q.Bind(o)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: failed to execute a one query for storages")
	}

	return o, nil
}

// AllP returns all Storage records from the query, and panics on error.
func (q storageQuery) AllP() StorageSlice {
	o, err := q.All()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// All returns all Storage records from the query.
func (q storageQuery) All() (StorageSlice, error) {
	var o []*Storage

	err := q.Bind(&o)
	if err != nil {
		return nil, errors.Wrap(err, "mdbmodels: failed to assign all query results to Storage slice")
	}

	return o, nil
}

// CountP returns the count of all Storage records in the query, and panics on error.
func (q storageQuery) CountP() int64 {
	c, err := q.Count()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return c
}

// Count returns the count of all Storage records in the query.
func (q storageQuery) Count() (int64, error) {
	var count int64

	queries.SetSelect(q.Query, nil)
	queries.SetCount(q.Query)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, "mdbmodels: failed to count storages rows")
	}

	return count, nil
}

// Exists checks if the row exists in the table, and panics on error.
func (q storageQuery) ExistsP() bool {
	e, err := q.Exists()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// Exists checks if the row exists in the table.
func (q storageQuery) Exists() (bool, error) {
	var count int64

	queries.SetCount(q.Query)
	queries.SetLimit(q.Query, 1)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: failed to check if storages exists")
	}

	return count > 0, nil
}

// FilesG retrieves all the file's files.
func (o *Storage) FilesG(mods ...qm.QueryMod) fileQuery {
	return o.Files(boil.GetDB(), mods...)
}

// Files retrieves all the file's files with an executor.
func (o *Storage) Files(exec boil.Executor, mods ...qm.QueryMod) fileQuery {
	var queryMods []qm.QueryMod
	if len(mods) != 0 {
		queryMods = append(queryMods, mods...)
	}

	queryMods = append(queryMods,
		qm.InnerJoin("\"files_storages\" on \"files\".\"id\" = \"files_storages\".\"file_id\""),
		qm.Where("\"files_storages\".\"storage_id\"=?", o.ID),
	)

	query := Files(exec, queryMods...)
	queries.SetFrom(query.Query, "\"files\"")

	if len(queries.GetSelect(query.Query)) == 0 {
		queries.SetSelect(query.Query, []string{"\"files\".*"})
	}

	return query
}

// LoadFiles allows an eager lookup of values, cached into the
// loaded structs of the objects.
func (storageL) LoadFiles(e boil.Executor, singular bool, maybeStorage interface{}) error {
	var slice []*Storage
	var object *Storage

	count := 1
	if singular {
		object = maybeStorage.(*Storage)
	} else {
		slice = *maybeStorage.(*[]*Storage)
		count = len(slice)
	}

	args := make([]interface{}, count)
	if singular {
		if object.R == nil {
			object.R = &storageR{}
		}
		args[0] = object.ID
	} else {
		for i, obj := range slice {
			if obj.R == nil {
				obj.R = &storageR{}
			}
			args[i] = obj.ID
		}
	}

	query := fmt.Sprintf(
		"select \"a\".*, \"b\".\"storage_id\" from \"files\" as \"a\" inner join \"files_storages\" as \"b\" on \"a\".\"id\" = \"b\".\"file_id\" where \"b\".\"storage_id\" in (%s)",
		strmangle.Placeholders(dialect.IndexPlaceholders, count, 1, 1),
	)
	if boil.DebugMode {
		fmt.Fprintf(boil.DebugWriter, "%s\n%v\n", query, args)
	}

	results, err := e.Query(query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to eager load files")
	}
	defer results.Close()

	var resultSlice []*File

	var localJoinCols []int64
	for results.Next() {
		one := new(File)
		var localJoinCol int64

		err = results.Scan(&one.ID, &one.UID, &one.Name, &one.Size, &one.Type, &one.SubType, &one.MimeType, &one.Sha1, &one.ContentUnitID, &one.CreatedAt, &one.Language, &one.BackupCount, &one.FirstBackupTime, &one.Properties, &one.ParentID, &one.FileCreatedAt, &one.Secure, &one.Published, &one.RemovedAt, &localJoinCol)
		if err = results.Err(); err != nil {
			return errors.Wrap(err, "failed to plebian-bind eager loaded slice files")
		}

		resultSlice = append(resultSlice, one)
		localJoinCols = append(localJoinCols, localJoinCol)
	}

	if err = results.Err(); err != nil {
		return errors.Wrap(err, "failed to plebian-bind eager loaded slice files")
	}

	if singular {
		object.R.Files = resultSlice
		return nil
	}

	for i, foreign := range resultSlice {
		localJoinCol := localJoinCols[i]
		for _, local := range slice {
			if local.ID == localJoinCol {
				local.R.Files = append(local.R.Files, foreign)
				break
			}
		}
	}

	return nil
}

// AddFilesG adds the given related objects to the existing relationships
// of the storage, optionally inserting them as new records.
// Appends related to o.R.Files.
// Sets related.R.Storages appropriately.
// Uses the global database handle.
func (o *Storage) AddFilesG(insert bool, related ...*File) error {
	return o.AddFiles(boil.GetDB(), insert, related...)
}

// AddFilesP adds the given related objects to the existing relationships
// of the storage, optionally inserting them as new records.
// Appends related to o.R.Files.
// Sets related.R.Storages appropriately.
// Panics on error.
func (o *Storage) AddFilesP(exec boil.Executor, insert bool, related ...*File) {
	if err := o.AddFiles(exec, insert, related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// AddFilesGP adds the given related objects to the existing relationships
// of the storage, optionally inserting them as new records.
// Appends related to o.R.Files.
// Sets related.R.Storages appropriately.
// Uses the global database handle and panics on error.
func (o *Storage) AddFilesGP(insert bool, related ...*File) {
	if err := o.AddFiles(boil.GetDB(), insert, related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// AddFiles adds the given related objects to the existing relationships
// of the storage, optionally inserting them as new records.
// Appends related to o.R.Files.
// Sets related.R.Storages appropriately.
func (o *Storage) AddFiles(exec boil.Executor, insert bool, related ...*File) error {
	var err error
	for _, rel := range related {
		if insert {
			if err = rel.Insert(exec); err != nil {
				return errors.Wrap(err, "failed to insert into foreign table")
			}
		}
	}

	for _, rel := range related {
		query := "insert into \"files_storages\" (\"storage_id\", \"file_id\") values ($1, $2)"
		values := []interface{}{o.ID, rel.ID}

		if boil.DebugMode {
			fmt.Fprintln(boil.DebugWriter, query)
			fmt.Fprintln(boil.DebugWriter, values)
		}

		_, err = exec.Exec(query, values...)
		if err != nil {
			return errors.Wrap(err, "failed to insert into join table")
		}
	}
	if o.R == nil {
		o.R = &storageR{
			Files: related,
		}
	} else {
		o.R.Files = append(o.R.Files, related...)
	}

	for _, rel := range related {
		if rel.R == nil {
			rel.R = &fileR{
				Storages: StorageSlice{o},
			}
		} else {
			rel.R.Storages = append(rel.R.Storages, o)
		}
	}
	return nil
}

// SetFilesG removes all previously related items of the
// storage replacing them completely with the passed
// in related items, optionally inserting them as new records.
// Sets o.R.Storages's Files accordingly.
// Replaces o.R.Files with related.
// Sets related.R.Storages's Files accordingly.
// Uses the global database handle.
func (o *Storage) SetFilesG(insert bool, related ...*File) error {
	return o.SetFiles(boil.GetDB(), insert, related...)
}

// SetFilesP removes all previously related items of the
// storage replacing them completely with the passed
// in related items, optionally inserting them as new records.
// Sets o.R.Storages's Files accordingly.
// Replaces o.R.Files with related.
// Sets related.R.Storages's Files accordingly.
// Panics on error.
func (o *Storage) SetFilesP(exec boil.Executor, insert bool, related ...*File) {
	if err := o.SetFiles(exec, insert, related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetFilesGP removes all previously related items of the
// storage replacing them completely with the passed
// in related items, optionally inserting them as new records.
// Sets o.R.Storages's Files accordingly.
// Replaces o.R.Files with related.
// Sets related.R.Storages's Files accordingly.
// Uses the global database handle and panics on error.
func (o *Storage) SetFilesGP(insert bool, related ...*File) {
	if err := o.SetFiles(boil.GetDB(), insert, related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetFiles removes all previously related items of the
// storage replacing them completely with the passed
// in related items, optionally inserting them as new records.
// Sets o.R.Storages's Files accordingly.
// Replaces o.R.Files with related.
// Sets related.R.Storages's Files accordingly.
func (o *Storage) SetFiles(exec boil.Executor, insert bool, related ...*File) error {
	query := "delete from \"files_storages\" where \"storage_id\" = $1"
	values := []interface{}{o.ID}
	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, query)
		fmt.Fprintln(boil.DebugWriter, values)
	}

	_, err := exec.Exec(query, values...)
	if err != nil {
		return errors.Wrap(err, "failed to remove relationships before set")
	}

	removeFilesFromStoragesSlice(o, related)
	if o.R != nil {
		o.R.Files = nil
	}
	return o.AddFiles(exec, insert, related...)
}

// RemoveFilesG relationships from objects passed in.
// Removes related items from R.Files (uses pointer comparison, removal does not keep order)
// Sets related.R.Storages.
// Uses the global database handle.
func (o *Storage) RemoveFilesG(related ...*File) error {
	return o.RemoveFiles(boil.GetDB(), related...)
}

// RemoveFilesP relationships from objects passed in.
// Removes related items from R.Files (uses pointer comparison, removal does not keep order)
// Sets related.R.Storages.
// Panics on error.
func (o *Storage) RemoveFilesP(exec boil.Executor, related ...*File) {
	if err := o.RemoveFiles(exec, related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// RemoveFilesGP relationships from objects passed in.
// Removes related items from R.Files (uses pointer comparison, removal does not keep order)
// Sets related.R.Storages.
// Uses the global database handle and panics on error.
func (o *Storage) RemoveFilesGP(related ...*File) {
	if err := o.RemoveFiles(boil.GetDB(), related...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// RemoveFiles relationships from objects passed in.
// Removes related items from R.Files (uses pointer comparison, removal does not keep order)
// Sets related.R.Storages.
func (o *Storage) RemoveFiles(exec boil.Executor, related ...*File) error {
	var err error
	query := fmt.Sprintf(
		"delete from \"files_storages\" where \"storage_id\" = $1 and \"file_id\" in (%s)",
		strmangle.Placeholders(dialect.IndexPlaceholders, len(related), 2, 1),
	)
	values := []interface{}{o.ID}
	for _, rel := range related {
		values = append(values, rel.ID)
	}

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, query)
		fmt.Fprintln(boil.DebugWriter, values)
	}

	_, err = exec.Exec(query, values...)
	if err != nil {
		return errors.Wrap(err, "failed to remove relationships before set")
	}
	removeFilesFromStoragesSlice(o, related)
	if o.R == nil {
		return nil
	}

	for _, rel := range related {
		for i, ri := range o.R.Files {
			if rel != ri {
				continue
			}

			ln := len(o.R.Files)
			if ln > 1 && i < ln-1 {
				o.R.Files[i] = o.R.Files[ln-1]
			}
			o.R.Files = o.R.Files[:ln-1]
			break
		}
	}

	return nil
}

func removeFilesFromStoragesSlice(o *Storage, related []*File) {
	for _, rel := range related {
		if rel.R == nil {
			continue
		}
		for i, ri := range rel.R.Storages {
			if o.ID != ri.ID {
				continue
			}

			ln := len(rel.R.Storages)
			if ln > 1 && i < ln-1 {
				rel.R.Storages[i] = rel.R.Storages[ln-1]
			}
			rel.R.Storages = rel.R.Storages[:ln-1]
			break
		}
	}
}

// StoragesG retrieves all records.
func StoragesG(mods ...qm.QueryMod) storageQuery {
	return Storages(boil.GetDB(), mods...)
}

// Storages retrieves all the records using an executor.
func Storages(exec boil.Executor, mods ...qm.QueryMod) storageQuery {
	mods = append(mods, qm.From("\"storages\""))
	return storageQuery{NewQuery(exec, mods...)}
}

// FindStorageG retrieves a single record by ID.
func FindStorageG(id int64, selectCols ...string) (*Storage, error) {
	return FindStorage(boil.GetDB(), id, selectCols...)
}

// FindStorageGP retrieves a single record by ID, and panics on error.
func FindStorageGP(id int64, selectCols ...string) *Storage {
	retobj, err := FindStorage(boil.GetDB(), id, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// FindStorage retrieves a single record by ID with an executor.
// If selectCols is empty Find will return all columns.
func FindStorage(exec boil.Executor, id int64, selectCols ...string) (*Storage, error) {
	storageObj := &Storage{}

	sel := "*"
	if len(selectCols) > 0 {
		sel = strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, selectCols), ",")
	}
	query := fmt.Sprintf(
		"select %s from \"storages\" where \"id\"=$1", sel,
	)

	q := queries.Raw(exec, query, id)

	err := q.Bind(storageObj)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: unable to select from storages")
	}

	return storageObj, nil
}

// FindStorageP retrieves a single record by ID with an executor, and panics on error.
func FindStorageP(exec boil.Executor, id int64, selectCols ...string) *Storage {
	retobj, err := FindStorage(exec, id, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// InsertG a single record. See Insert for whitelist behavior description.
func (o *Storage) InsertG(whitelist ...string) error {
	return o.Insert(boil.GetDB(), whitelist...)
}

// InsertGP a single record, and panics on error. See Insert for whitelist
// behavior description.
func (o *Storage) InsertGP(whitelist ...string) {
	if err := o.Insert(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// InsertP a single record using an executor, and panics on error. See Insert
// for whitelist behavior description.
func (o *Storage) InsertP(exec boil.Executor, whitelist ...string) {
	if err := o.Insert(exec, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Insert a single record using an executor.
// Whitelist behavior: If a whitelist is provided, only those columns supplied are inserted
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns without a default value are included (i.e. name, age)
// - All columns with a default, but non-zero are included (i.e. health = 75)
func (o *Storage) Insert(exec boil.Executor, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no storages provided for insertion")
	}

	var err error

	nzDefaults := queries.NonZeroDefaultSet(storageColumnsWithDefault, o)

	key := makeCacheKey(whitelist, nzDefaults)
	storageInsertCacheMut.RLock()
	cache, cached := storageInsertCache[key]
	storageInsertCacheMut.RUnlock()

	if !cached {
		wl, returnColumns := strmangle.InsertColumnSet(
			storageColumns,
			storageColumnsWithDefault,
			storageColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)

		cache.valueMapping, err = queries.BindMapping(storageType, storageMapping, wl)
		if err != nil {
			return err
		}
		cache.retMapping, err = queries.BindMapping(storageType, storageMapping, returnColumns)
		if err != nil {
			return err
		}
		if len(wl) != 0 {
			cache.query = fmt.Sprintf("INSERT INTO \"storages\" (\"%s\") %%sVALUES (%s)%%s", strings.Join(wl, "\",\""), strmangle.Placeholders(dialect.IndexPlaceholders, len(wl), 1, 1))
		} else {
			cache.query = "INSERT INTO \"storages\" DEFAULT VALUES"
		}

		var queryOutput, queryReturning string

		if len(cache.retMapping) != 0 {
			queryReturning = fmt.Sprintf(" RETURNING \"%s\"", strings.Join(returnColumns, "\",\""))
		}

		if len(wl) != 0 {
			cache.query = fmt.Sprintf(cache.query, queryOutput, queryReturning)
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
		return errors.Wrap(err, "mdbmodels: unable to insert into storages")
	}

	if !cached {
		storageInsertCacheMut.Lock()
		storageInsertCache[key] = cache
		storageInsertCacheMut.Unlock()
	}

	return nil
}

// UpdateG a single Storage record. See Update for
// whitelist behavior description.
func (o *Storage) UpdateG(whitelist ...string) error {
	return o.Update(boil.GetDB(), whitelist...)
}

// UpdateGP a single Storage record.
// UpdateGP takes a whitelist of column names that should be updated.
// Panics on error. See Update for whitelist behavior description.
func (o *Storage) UpdateGP(whitelist ...string) {
	if err := o.Update(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateP uses an executor to update the Storage, and panics on error.
// See Update for whitelist behavior description.
func (o *Storage) UpdateP(exec boil.Executor, whitelist ...string) {
	err := o.Update(exec, whitelist...)
	if err != nil {
		panic(boil.WrapErr(err))
	}
}

// Update uses an executor to update the Storage.
// Whitelist behavior: If a whitelist is provided, only the columns given are updated.
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns are inferred to start with
// - All primary keys are subtracted from this set
// Update does not automatically update the record in case of default values. Use .Reload()
// to refresh the records.
func (o *Storage) Update(exec boil.Executor, whitelist ...string) error {
	var err error
	key := makeCacheKey(whitelist, nil)
	storageUpdateCacheMut.RLock()
	cache, cached := storageUpdateCache[key]
	storageUpdateCacheMut.RUnlock()

	if !cached {
		wl := strmangle.UpdateColumnSet(
			storageColumns,
			storagePrimaryKeyColumns,
			whitelist,
		)

		if len(wl) == 0 {
			return errors.New("mdbmodels: unable to update storages, could not build whitelist")
		}

		cache.query = fmt.Sprintf("UPDATE \"storages\" SET %s WHERE %s",
			strmangle.SetParamNames("\"", "\"", 1, wl),
			strmangle.WhereClause("\"", "\"", len(wl)+1, storagePrimaryKeyColumns),
		)
		cache.valueMapping, err = queries.BindMapping(storageType, storageMapping, append(wl, storagePrimaryKeyColumns...))
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
		return errors.Wrap(err, "mdbmodels: unable to update storages row")
	}

	if !cached {
		storageUpdateCacheMut.Lock()
		storageUpdateCache[key] = cache
		storageUpdateCacheMut.Unlock()
	}

	return nil
}

// UpdateAllP updates all rows with matching column names, and panics on error.
func (q storageQuery) UpdateAllP(cols M) {
	if err := q.UpdateAll(cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values.
func (q storageQuery) UpdateAll(cols M) error {
	queries.SetUpdate(q.Query, cols)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all for storages")
	}

	return nil
}

// UpdateAllG updates all rows with the specified column values.
func (o StorageSlice) UpdateAllG(cols M) error {
	return o.UpdateAll(boil.GetDB(), cols)
}

// UpdateAllGP updates all rows with the specified column values, and panics on error.
func (o StorageSlice) UpdateAllGP(cols M) {
	if err := o.UpdateAll(boil.GetDB(), cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAllP updates all rows with the specified column values, and panics on error.
func (o StorageSlice) UpdateAllP(exec boil.Executor, cols M) {
	if err := o.UpdateAll(exec, cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values, using an executor.
func (o StorageSlice) UpdateAll(exec boil.Executor, cols M) error {
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
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), storagePrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf("UPDATE \"storages\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, colNames),
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), len(colNames)+1, storagePrimaryKeyColumns, len(o)))

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all in storage slice")
	}

	return nil
}

// UpsertG attempts an insert, and does an update or ignore on conflict.
func (o *Storage) UpsertG(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	return o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...)
}

// UpsertGP attempts an insert, and does an update or ignore on conflict. Panics on error.
func (o *Storage) UpsertGP(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpsertP attempts an insert using an executor, and does an update or ignore on conflict.
// UpsertP panics on error.
func (o *Storage) UpsertP(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(exec, updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Upsert attempts an insert using an executor, and does an update or ignore on conflict.
func (o *Storage) Upsert(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no storages provided for upsert")
	}

	nzDefaults := queries.NonZeroDefaultSet(storageColumnsWithDefault, o)

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

	storageUpsertCacheMut.RLock()
	cache, cached := storageUpsertCache[key]
	storageUpsertCacheMut.RUnlock()

	var err error

	if !cached {
		insert, ret := strmangle.InsertColumnSet(
			storageColumns,
			storageColumnsWithDefault,
			storageColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)

		update := strmangle.UpdateColumnSet(
			storageColumns,
			storagePrimaryKeyColumns,
			updateColumns,
		)
		if len(update) == 0 {
			return errors.New("mdbmodels: unable to upsert storages, could not build update column list")
		}

		conflict := conflictColumns
		if len(conflict) == 0 {
			conflict = make([]string, len(storagePrimaryKeyColumns))
			copy(conflict, storagePrimaryKeyColumns)
		}
		cache.query = queries.BuildUpsertQueryPostgres(dialect, "\"storages\"", updateOnConflict, ret, update, conflict, insert)

		cache.valueMapping, err = queries.BindMapping(storageType, storageMapping, insert)
		if err != nil {
			return err
		}
		if len(ret) != 0 {
			cache.retMapping, err = queries.BindMapping(storageType, storageMapping, ret)
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
		return errors.Wrap(err, "mdbmodels: unable to upsert storages")
	}

	if !cached {
		storageUpsertCacheMut.Lock()
		storageUpsertCache[key] = cache
		storageUpsertCacheMut.Unlock()
	}

	return nil
}

// DeleteP deletes a single Storage record with an executor.
// DeleteP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *Storage) DeleteP(exec boil.Executor) {
	if err := o.Delete(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteG deletes a single Storage record.
// DeleteG will match against the primary key column to find the record to delete.
func (o *Storage) DeleteG() error {
	if o == nil {
		return errors.New("mdbmodels: no Storage provided for deletion")
	}

	return o.Delete(boil.GetDB())
}

// DeleteGP deletes a single Storage record.
// DeleteGP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *Storage) DeleteGP() {
	if err := o.DeleteG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Delete deletes a single Storage record with an executor.
// Delete will match against the primary key column to find the record to delete.
func (o *Storage) Delete(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no Storage provided for delete")
	}

	args := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), storagePrimaryKeyMapping)
	sql := "DELETE FROM \"storages\" WHERE \"id\"=$1"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete from storages")
	}

	return nil
}

// DeleteAllP deletes all rows, and panics on error.
func (q storageQuery) DeleteAllP() {
	if err := q.DeleteAll(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all matching rows.
func (q storageQuery) DeleteAll() error {
	if q.Query == nil {
		return errors.New("mdbmodels: no storageQuery provided for delete all")
	}

	queries.SetDelete(q.Query)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from storages")
	}

	return nil
}

// DeleteAllGP deletes all rows in the slice, and panics on error.
func (o StorageSlice) DeleteAllGP() {
	if err := o.DeleteAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAllG deletes all rows in the slice.
func (o StorageSlice) DeleteAllG() error {
	if o == nil {
		return errors.New("mdbmodels: no Storage slice provided for delete all")
	}
	return o.DeleteAll(boil.GetDB())
}

// DeleteAllP deletes all rows in the slice, using an executor, and panics on error.
func (o StorageSlice) DeleteAllP(exec boil.Executor) {
	if err := o.DeleteAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all rows in the slice, using an executor.
func (o StorageSlice) DeleteAll(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no Storage slice provided for delete all")
	}

	if len(o) == 0 {
		return nil
	}

	var args []interface{}
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), storagePrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "DELETE FROM \"storages\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, storagePrimaryKeyColumns, len(o))

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from storage slice")
	}

	return nil
}

// ReloadGP refetches the object from the database and panics on error.
func (o *Storage) ReloadGP() {
	if err := o.ReloadG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadP refetches the object from the database with an executor. Panics on error.
func (o *Storage) ReloadP(exec boil.Executor) {
	if err := o.Reload(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadG refetches the object from the database using the primary keys.
func (o *Storage) ReloadG() error {
	if o == nil {
		return errors.New("mdbmodels: no Storage provided for reload")
	}

	return o.Reload(boil.GetDB())
}

// Reload refetches the object from the database
// using the primary keys with an executor.
func (o *Storage) Reload(exec boil.Executor) error {
	ret, err := FindStorage(exec, o.ID)
	if err != nil {
		return err
	}

	*o = *ret
	return nil
}

// ReloadAllGP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *StorageSlice) ReloadAllGP() {
	if err := o.ReloadAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *StorageSlice) ReloadAllP(exec boil.Executor) {
	if err := o.ReloadAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllG refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *StorageSlice) ReloadAllG() error {
	if o == nil {
		return errors.New("mdbmodels: empty StorageSlice provided for reload all")
	}

	return o.ReloadAll(boil.GetDB())
}

// ReloadAll refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *StorageSlice) ReloadAll(exec boil.Executor) error {
	if o == nil || len(*o) == 0 {
		return nil
	}

	storages := StorageSlice{}
	var args []interface{}
	for _, obj := range *o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), storagePrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "SELECT \"storages\".* FROM \"storages\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, storagePrimaryKeyColumns, len(*o))

	q := queries.Raw(exec, sql, args...)

	err := q.Bind(&storages)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to reload all in StorageSlice")
	}

	*o = storages

	return nil
}

// StorageExists checks if the Storage row exists.
func StorageExists(exec boil.Executor, id int64) (bool, error) {
	var exists bool
	sql := "select exists(select 1 from \"storages\" where \"id\"=$1 limit 1)"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, id)
	}

	row := exec.QueryRow(sql, id)

	err := row.Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: unable to check if storages exists")
	}

	return exists, nil
}

// StorageExistsG checks if the Storage row exists.
func StorageExistsG(id int64) (bool, error) {
	return StorageExists(boil.GetDB(), id)
}

// StorageExistsGP checks if the Storage row exists. Panics on error.
func StorageExistsGP(id int64) bool {
	e, err := StorageExists(boil.GetDB(), id)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// StorageExistsP checks if the Storage row exists. Panics on error.
func StorageExistsP(exec boil.Executor, id int64) bool {
	e, err := StorageExists(exec, id)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}
