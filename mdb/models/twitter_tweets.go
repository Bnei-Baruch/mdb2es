// Code generated by SQLBoiler (https://github.com/volatiletech/sqlboiler). DO NOT EDIT.
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
	"gopkg.in/volatiletech/null.v6"
)

// TwitterTweet is an object representing the database table.
type TwitterTweet struct {
	ID        int64     `boil:"id" json:"id" toml:"id" yaml:"id"`
	UserID    int64     `boil:"user_id" json:"user_id" toml:"user_id" yaml:"user_id"`
	TwitterID string    `boil:"twitter_id" json:"twitter_id" toml:"twitter_id" yaml:"twitter_id"`
	FullText  string    `boil:"full_text" json:"full_text" toml:"full_text" yaml:"full_text"`
	TweetAt   time.Time `boil:"tweet_at" json:"tweet_at" toml:"tweet_at" yaml:"tweet_at"`
	Raw       null.JSON `boil:"raw" json:"raw,omitempty" toml:"raw" yaml:"raw,omitempty"`
	CreatedAt time.Time `boil:"created_at" json:"created_at" toml:"created_at" yaml:"created_at"`

	R *twitterTweetR `boil:"-" json:"-" toml:"-" yaml:"-"`
	L twitterTweetL  `boil:"-" json:"-" toml:"-" yaml:"-"`
}

var TwitterTweetColumns = struct {
	ID        string
	UserID    string
	TwitterID string
	FullText  string
	TweetAt   string
	Raw       string
	CreatedAt string
}{
	ID:        "id",
	UserID:    "user_id",
	TwitterID: "twitter_id",
	FullText:  "full_text",
	TweetAt:   "tweet_at",
	Raw:       "raw",
	CreatedAt: "created_at",
}

// twitterTweetR is where relationships are stored.
type twitterTweetR struct {
	User *TwitterUser
}

// twitterTweetL is where Load methods for each relationship are stored.
type twitterTweetL struct{}

var (
	twitterTweetColumns               = []string{"id", "user_id", "twitter_id", "full_text", "tweet_at", "raw", "created_at"}
	twitterTweetColumnsWithoutDefault = []string{"user_id", "twitter_id", "full_text", "tweet_at", "raw"}
	twitterTweetColumnsWithDefault    = []string{"id", "created_at"}
	twitterTweetPrimaryKeyColumns     = []string{"id"}
)

type (
	// TwitterTweetSlice is an alias for a slice of pointers to TwitterTweet.
	// This should generally be used opposed to []TwitterTweet.
	TwitterTweetSlice []*TwitterTweet

	twitterTweetQuery struct {
		*queries.Query
	}
)

// Cache for insert, update and upsert
var (
	twitterTweetType                 = reflect.TypeOf(&TwitterTweet{})
	twitterTweetMapping              = queries.MakeStructMapping(twitterTweetType)
	twitterTweetPrimaryKeyMapping, _ = queries.BindMapping(twitterTweetType, twitterTweetMapping, twitterTweetPrimaryKeyColumns)
	twitterTweetInsertCacheMut       sync.RWMutex
	twitterTweetInsertCache          = make(map[string]insertCache)
	twitterTweetUpdateCacheMut       sync.RWMutex
	twitterTweetUpdateCache          = make(map[string]updateCache)
	twitterTweetUpsertCacheMut       sync.RWMutex
	twitterTweetUpsertCache          = make(map[string]insertCache)
)

var (
	// Force time package dependency for automated UpdatedAt/CreatedAt.
	_ = time.Second
	// Force bytes in case of primary key column that uses []byte (for relationship compares)
	_ = bytes.MinRead
)

// OneP returns a single twitterTweet record from the query, and panics on error.
func (q twitterTweetQuery) OneP() *TwitterTweet {
	o, err := q.One()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// One returns a single twitterTweet record from the query.
func (q twitterTweetQuery) One() (*TwitterTweet, error) {
	o := &TwitterTweet{}

	queries.SetLimit(q.Query, 1)

	err := q.Bind(o)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: failed to execute a one query for twitter_tweets")
	}

	return o, nil
}

// AllP returns all TwitterTweet records from the query, and panics on error.
func (q twitterTweetQuery) AllP() TwitterTweetSlice {
	o, err := q.All()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// All returns all TwitterTweet records from the query.
func (q twitterTweetQuery) All() (TwitterTweetSlice, error) {
	var o []*TwitterTweet

	err := q.Bind(&o)
	if err != nil {
		return nil, errors.Wrap(err, "mdbmodels: failed to assign all query results to TwitterTweet slice")
	}

	return o, nil
}

// CountP returns the count of all TwitterTweet records in the query, and panics on error.
func (q twitterTweetQuery) CountP() int64 {
	c, err := q.Count()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return c
}

// Count returns the count of all TwitterTweet records in the query.
func (q twitterTweetQuery) Count() (int64, error) {
	var count int64

	queries.SetSelect(q.Query, nil)
	queries.SetCount(q.Query)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, "mdbmodels: failed to count twitter_tweets rows")
	}

	return count, nil
}

// Exists checks if the row exists in the table, and panics on error.
func (q twitterTweetQuery) ExistsP() bool {
	e, err := q.Exists()
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// Exists checks if the row exists in the table.
func (q twitterTweetQuery) Exists() (bool, error) {
	var count int64

	queries.SetCount(q.Query)
	queries.SetLimit(q.Query, 1)

	err := q.Query.QueryRow().Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: failed to check if twitter_tweets exists")
	}

	return count > 0, nil
}

// UserG pointed to by the foreign key.
func (o *TwitterTweet) UserG(mods ...qm.QueryMod) twitterUserQuery {
	return o.User(boil.GetDB(), mods...)
}

// User pointed to by the foreign key.
func (o *TwitterTweet) User(exec boil.Executor, mods ...qm.QueryMod) twitterUserQuery {
	queryMods := []qm.QueryMod{
		qm.Where("id=?", o.UserID),
	}

	queryMods = append(queryMods, mods...)

	query := TwitterUsers(exec, queryMods...)
	queries.SetFrom(query.Query, "\"twitter_users\"")

	return query
} // LoadUser allows an eager lookup of values, cached into the
// loaded structs of the objects.
func (twitterTweetL) LoadUser(e boil.Executor, singular bool, maybeTwitterTweet interface{}) error {
	var slice []*TwitterTweet
	var object *TwitterTweet

	count := 1
	if singular {
		object = maybeTwitterTweet.(*TwitterTweet)
	} else {
		slice = *maybeTwitterTweet.(*[]*TwitterTweet)
		count = len(slice)
	}

	args := make([]interface{}, count)
	if singular {
		if object.R == nil {
			object.R = &twitterTweetR{}
		}
		args[0] = object.UserID
	} else {
		for i, obj := range slice {
			if obj.R == nil {
				obj.R = &twitterTweetR{}
			}
			args[i] = obj.UserID
		}
	}

	query := fmt.Sprintf(
		"select * from \"twitter_users\" where \"id\" in (%s)",
		strmangle.Placeholders(dialect.IndexPlaceholders, count, 1, 1),
	)

	if boil.DebugMode {
		fmt.Fprintf(boil.DebugWriter, "%s\n%v\n", query, args)
	}

	results, err := e.Query(query, args...)
	if err != nil {
		return errors.Wrap(err, "failed to eager load TwitterUser")
	}
	defer results.Close()

	var resultSlice []*TwitterUser
	if err = queries.Bind(results, &resultSlice); err != nil {
		return errors.Wrap(err, "failed to bind eager loaded slice TwitterUser")
	}

	if len(resultSlice) == 0 {
		return nil
	}

	if singular {
		object.R.User = resultSlice[0]
		return nil
	}

	for _, local := range slice {
		for _, foreign := range resultSlice {
			if local.UserID == foreign.ID {
				local.R.User = foreign
				break
			}
		}
	}

	return nil
}

// SetUserG of the twitter_tweet to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserTwitterTweets.
// Uses the global database handle.
func (o *TwitterTweet) SetUserG(insert bool, related *TwitterUser) error {
	return o.SetUser(boil.GetDB(), insert, related)
}

// SetUserP of the twitter_tweet to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserTwitterTweets.
// Panics on error.
func (o *TwitterTweet) SetUserP(exec boil.Executor, insert bool, related *TwitterUser) {
	if err := o.SetUser(exec, insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetUserGP of the twitter_tweet to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserTwitterTweets.
// Uses the global database handle and panics on error.
func (o *TwitterTweet) SetUserGP(insert bool, related *TwitterUser) {
	if err := o.SetUser(boil.GetDB(), insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetUser of the twitter_tweet to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserTwitterTweets.
func (o *TwitterTweet) SetUser(exec boil.Executor, insert bool, related *TwitterUser) error {
	var err error
	if insert {
		if err = related.Insert(exec); err != nil {
			return errors.Wrap(err, "failed to insert into foreign table")
		}
	}

	updateQuery := fmt.Sprintf(
		"UPDATE \"twitter_tweets\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, []string{"user_id"}),
		strmangle.WhereClause("\"", "\"", 2, twitterTweetPrimaryKeyColumns),
	)
	values := []interface{}{related.ID, o.ID}

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, updateQuery)
		fmt.Fprintln(boil.DebugWriter, values)
	}

	if _, err = exec.Exec(updateQuery, values...); err != nil {
		return errors.Wrap(err, "failed to update local table")
	}

	o.UserID = related.ID

	if o.R == nil {
		o.R = &twitterTweetR{
			User: related,
		}
	} else {
		o.R.User = related
	}

	if related.R == nil {
		related.R = &twitterUserR{
			UserTwitterTweets: TwitterTweetSlice{o},
		}
	} else {
		related.R.UserTwitterTweets = append(related.R.UserTwitterTweets, o)
	}

	return nil
}

// TwitterTweetsG retrieves all records.
func TwitterTweetsG(mods ...qm.QueryMod) twitterTweetQuery {
	return TwitterTweets(boil.GetDB(), mods...)
}

// TwitterTweets retrieves all the records using an executor.
func TwitterTweets(exec boil.Executor, mods ...qm.QueryMod) twitterTweetQuery {
	mods = append(mods, qm.From("\"twitter_tweets\""))
	return twitterTweetQuery{NewQuery(exec, mods...)}
}

// FindTwitterTweetG retrieves a single record by ID.
func FindTwitterTweetG(id int64, selectCols ...string) (*TwitterTweet, error) {
	return FindTwitterTweet(boil.GetDB(), id, selectCols...)
}

// FindTwitterTweetGP retrieves a single record by ID, and panics on error.
func FindTwitterTweetGP(id int64, selectCols ...string) *TwitterTweet {
	retobj, err := FindTwitterTweet(boil.GetDB(), id, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// FindTwitterTweet retrieves a single record by ID with an executor.
// If selectCols is empty Find will return all columns.
func FindTwitterTweet(exec boil.Executor, id int64, selectCols ...string) (*TwitterTweet, error) {
	twitterTweetObj := &TwitterTweet{}

	sel := "*"
	if len(selectCols) > 0 {
		sel = strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, selectCols), ",")
	}
	query := fmt.Sprintf(
		"select %s from \"twitter_tweets\" where \"id\"=$1", sel,
	)

	q := queries.Raw(exec, query, id)

	err := q.Bind(twitterTweetObj)
	if err != nil {
		if errors.Cause(err) == sql.ErrNoRows {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "mdbmodels: unable to select from twitter_tweets")
	}

	return twitterTweetObj, nil
}

// FindTwitterTweetP retrieves a single record by ID with an executor, and panics on error.
func FindTwitterTweetP(exec boil.Executor, id int64, selectCols ...string) *TwitterTweet {
	retobj, err := FindTwitterTweet(exec, id, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// InsertG a single record. See Insert for whitelist behavior description.
func (o *TwitterTweet) InsertG(whitelist ...string) error {
	return o.Insert(boil.GetDB(), whitelist...)
}

// InsertGP a single record, and panics on error. See Insert for whitelist
// behavior description.
func (o *TwitterTweet) InsertGP(whitelist ...string) {
	if err := o.Insert(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// InsertP a single record using an executor, and panics on error. See Insert
// for whitelist behavior description.
func (o *TwitterTweet) InsertP(exec boil.Executor, whitelist ...string) {
	if err := o.Insert(exec, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Insert a single record using an executor.
// Whitelist behavior: If a whitelist is provided, only those columns supplied are inserted
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns without a default value are included (i.e. name, age)
// - All columns with a default, but non-zero are included (i.e. health = 75)
func (o *TwitterTweet) Insert(exec boil.Executor, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no twitter_tweets provided for insertion")
	}

	var err error

	nzDefaults := queries.NonZeroDefaultSet(twitterTweetColumnsWithDefault, o)

	key := makeCacheKey(whitelist, nzDefaults)
	twitterTweetInsertCacheMut.RLock()
	cache, cached := twitterTweetInsertCache[key]
	twitterTweetInsertCacheMut.RUnlock()

	if !cached {
		wl, returnColumns := strmangle.InsertColumnSet(
			twitterTweetColumns,
			twitterTweetColumnsWithDefault,
			twitterTweetColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)

		cache.valueMapping, err = queries.BindMapping(twitterTweetType, twitterTweetMapping, wl)
		if err != nil {
			return err
		}
		cache.retMapping, err = queries.BindMapping(twitterTweetType, twitterTweetMapping, returnColumns)
		if err != nil {
			return err
		}
		if len(wl) != 0 {
			cache.query = fmt.Sprintf("INSERT INTO \"twitter_tweets\" (\"%s\") %%sVALUES (%s)%%s", strings.Join(wl, "\",\""), strmangle.Placeholders(dialect.IndexPlaceholders, len(wl), 1, 1))
		} else {
			cache.query = "INSERT INTO \"twitter_tweets\" DEFAULT VALUES"
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
		return errors.Wrap(err, "mdbmodels: unable to insert into twitter_tweets")
	}

	if !cached {
		twitterTweetInsertCacheMut.Lock()
		twitterTweetInsertCache[key] = cache
		twitterTweetInsertCacheMut.Unlock()
	}

	return nil
}

// UpdateG a single TwitterTweet record. See Update for
// whitelist behavior description.
func (o *TwitterTweet) UpdateG(whitelist ...string) error {
	return o.Update(boil.GetDB(), whitelist...)
}

// UpdateGP a single TwitterTweet record.
// UpdateGP takes a whitelist of column names that should be updated.
// Panics on error. See Update for whitelist behavior description.
func (o *TwitterTweet) UpdateGP(whitelist ...string) {
	if err := o.Update(boil.GetDB(), whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateP uses an executor to update the TwitterTweet, and panics on error.
// See Update for whitelist behavior description.
func (o *TwitterTweet) UpdateP(exec boil.Executor, whitelist ...string) {
	err := o.Update(exec, whitelist...)
	if err != nil {
		panic(boil.WrapErr(err))
	}
}

// Update uses an executor to update the TwitterTweet.
// Whitelist behavior: If a whitelist is provided, only the columns given are updated.
// No whitelist behavior: Without a whitelist, columns are inferred by the following rules:
// - All columns are inferred to start with
// - All primary keys are subtracted from this set
// Update does not automatically update the record in case of default values. Use .Reload()
// to refresh the records.
func (o *TwitterTweet) Update(exec boil.Executor, whitelist ...string) error {
	var err error
	key := makeCacheKey(whitelist, nil)
	twitterTweetUpdateCacheMut.RLock()
	cache, cached := twitterTweetUpdateCache[key]
	twitterTweetUpdateCacheMut.RUnlock()

	if !cached {
		wl := strmangle.UpdateColumnSet(
			twitterTweetColumns,
			twitterTweetPrimaryKeyColumns,
			whitelist,
		)

		if len(wl) == 0 {
			return errors.New("mdbmodels: unable to update twitter_tweets, could not build whitelist")
		}

		cache.query = fmt.Sprintf("UPDATE \"twitter_tweets\" SET %s WHERE %s",
			strmangle.SetParamNames("\"", "\"", 1, wl),
			strmangle.WhereClause("\"", "\"", len(wl)+1, twitterTweetPrimaryKeyColumns),
		)
		cache.valueMapping, err = queries.BindMapping(twitterTweetType, twitterTweetMapping, append(wl, twitterTweetPrimaryKeyColumns...))
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
		return errors.Wrap(err, "mdbmodels: unable to update twitter_tweets row")
	}

	if !cached {
		twitterTweetUpdateCacheMut.Lock()
		twitterTweetUpdateCache[key] = cache
		twitterTweetUpdateCacheMut.Unlock()
	}

	return nil
}

// UpdateAllP updates all rows with matching column names, and panics on error.
func (q twitterTweetQuery) UpdateAllP(cols M) {
	if err := q.UpdateAll(cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values.
func (q twitterTweetQuery) UpdateAll(cols M) error {
	queries.SetUpdate(q.Query, cols)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all for twitter_tweets")
	}

	return nil
}

// UpdateAllG updates all rows with the specified column values.
func (o TwitterTweetSlice) UpdateAllG(cols M) error {
	return o.UpdateAll(boil.GetDB(), cols)
}

// UpdateAllGP updates all rows with the specified column values, and panics on error.
func (o TwitterTweetSlice) UpdateAllGP(cols M) {
	if err := o.UpdateAll(boil.GetDB(), cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAllP updates all rows with the specified column values, and panics on error.
func (o TwitterTweetSlice) UpdateAllP(exec boil.Executor, cols M) {
	if err := o.UpdateAll(exec, cols); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpdateAll updates all rows with the specified column values, using an executor.
func (o TwitterTweetSlice) UpdateAll(exec boil.Executor, cols M) error {
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
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), twitterTweetPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf("UPDATE \"twitter_tweets\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, colNames),
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), len(colNames)+1, twitterTweetPrimaryKeyColumns, len(o)))

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to update all in twitterTweet slice")
	}

	return nil
}

// UpsertG attempts an insert, and does an update or ignore on conflict.
func (o *TwitterTweet) UpsertG(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	return o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...)
}

// UpsertGP attempts an insert, and does an update or ignore on conflict. Panics on error.
func (o *TwitterTweet) UpsertGP(updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(boil.GetDB(), updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// UpsertP attempts an insert using an executor, and does an update or ignore on conflict.
// UpsertP panics on error.
func (o *TwitterTweet) UpsertP(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) {
	if err := o.Upsert(exec, updateOnConflict, conflictColumns, updateColumns, whitelist...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Upsert attempts an insert using an executor, and does an update or ignore on conflict.
func (o *TwitterTweet) Upsert(exec boil.Executor, updateOnConflict bool, conflictColumns []string, updateColumns []string, whitelist ...string) error {
	if o == nil {
		return errors.New("mdbmodels: no twitter_tweets provided for upsert")
	}

	nzDefaults := queries.NonZeroDefaultSet(twitterTweetColumnsWithDefault, o)

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

	twitterTweetUpsertCacheMut.RLock()
	cache, cached := twitterTweetUpsertCache[key]
	twitterTweetUpsertCacheMut.RUnlock()

	var err error

	if !cached {
		insert, ret := strmangle.InsertColumnSet(
			twitterTweetColumns,
			twitterTweetColumnsWithDefault,
			twitterTweetColumnsWithoutDefault,
			nzDefaults,
			whitelist,
		)

		update := strmangle.UpdateColumnSet(
			twitterTweetColumns,
			twitterTweetPrimaryKeyColumns,
			updateColumns,
		)
		if len(update) == 0 {
			return errors.New("mdbmodels: unable to upsert twitter_tweets, could not build update column list")
		}

		conflict := conflictColumns
		if len(conflict) == 0 {
			conflict = make([]string, len(twitterTweetPrimaryKeyColumns))
			copy(conflict, twitterTweetPrimaryKeyColumns)
		}
		cache.query = queries.BuildUpsertQueryPostgres(dialect, "\"twitter_tweets\"", updateOnConflict, ret, update, conflict, insert)

		cache.valueMapping, err = queries.BindMapping(twitterTweetType, twitterTweetMapping, insert)
		if err != nil {
			return err
		}
		if len(ret) != 0 {
			cache.retMapping, err = queries.BindMapping(twitterTweetType, twitterTweetMapping, ret)
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
		return errors.Wrap(err, "mdbmodels: unable to upsert twitter_tweets")
	}

	if !cached {
		twitterTweetUpsertCacheMut.Lock()
		twitterTweetUpsertCache[key] = cache
		twitterTweetUpsertCacheMut.Unlock()
	}

	return nil
}

// DeleteP deletes a single TwitterTweet record with an executor.
// DeleteP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *TwitterTweet) DeleteP(exec boil.Executor) {
	if err := o.Delete(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteG deletes a single TwitterTweet record.
// DeleteG will match against the primary key column to find the record to delete.
func (o *TwitterTweet) DeleteG() error {
	if o == nil {
		return errors.New("mdbmodels: no TwitterTweet provided for deletion")
	}

	return o.Delete(boil.GetDB())
}

// DeleteGP deletes a single TwitterTweet record.
// DeleteGP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *TwitterTweet) DeleteGP() {
	if err := o.DeleteG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Delete deletes a single TwitterTweet record with an executor.
// Delete will match against the primary key column to find the record to delete.
func (o *TwitterTweet) Delete(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no TwitterTweet provided for delete")
	}

	args := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), twitterTweetPrimaryKeyMapping)
	sql := "DELETE FROM \"twitter_tweets\" WHERE \"id\"=$1"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args...)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete from twitter_tweets")
	}

	return nil
}

// DeleteAllP deletes all rows, and panics on error.
func (q twitterTweetQuery) DeleteAllP() {
	if err := q.DeleteAll(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all matching rows.
func (q twitterTweetQuery) DeleteAll() error {
	if q.Query == nil {
		return errors.New("mdbmodels: no twitterTweetQuery provided for delete all")
	}

	queries.SetDelete(q.Query)

	_, err := q.Query.Exec()
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from twitter_tweets")
	}

	return nil
}

// DeleteAllGP deletes all rows in the slice, and panics on error.
func (o TwitterTweetSlice) DeleteAllGP() {
	if err := o.DeleteAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAllG deletes all rows in the slice.
func (o TwitterTweetSlice) DeleteAllG() error {
	if o == nil {
		return errors.New("mdbmodels: no TwitterTweet slice provided for delete all")
	}
	return o.DeleteAll(boil.GetDB())
}

// DeleteAllP deletes all rows in the slice, using an executor, and panics on error.
func (o TwitterTweetSlice) DeleteAllP(exec boil.Executor) {
	if err := o.DeleteAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// DeleteAll deletes all rows in the slice, using an executor.
func (o TwitterTweetSlice) DeleteAll(exec boil.Executor) error {
	if o == nil {
		return errors.New("mdbmodels: no TwitterTweet slice provided for delete all")
	}

	if len(o) == 0 {
		return nil
	}

	var args []interface{}
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), twitterTweetPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "DELETE FROM \"twitter_tweets\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, twitterTweetPrimaryKeyColumns, len(o))

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, args)
	}

	_, err := exec.Exec(sql, args...)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to delete all from twitterTweet slice")
	}

	return nil
}

// ReloadGP refetches the object from the database and panics on error.
func (o *TwitterTweet) ReloadGP() {
	if err := o.ReloadG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadP refetches the object from the database with an executor. Panics on error.
func (o *TwitterTweet) ReloadP(exec boil.Executor) {
	if err := o.Reload(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadG refetches the object from the database using the primary keys.
func (o *TwitterTweet) ReloadG() error {
	if o == nil {
		return errors.New("mdbmodels: no TwitterTweet provided for reload")
	}

	return o.Reload(boil.GetDB())
}

// Reload refetches the object from the database
// using the primary keys with an executor.
func (o *TwitterTweet) Reload(exec boil.Executor) error {
	ret, err := FindTwitterTweet(exec, o.ID)
	if err != nil {
		return err
	}

	*o = *ret
	return nil
}

// ReloadAllGP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *TwitterTweetSlice) ReloadAllGP() {
	if err := o.ReloadAllG(); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *TwitterTweetSlice) ReloadAllP(exec boil.Executor) {
	if err := o.ReloadAll(exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAllG refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *TwitterTweetSlice) ReloadAllG() error {
	if o == nil {
		return errors.New("mdbmodels: empty TwitterTweetSlice provided for reload all")
	}

	return o.ReloadAll(boil.GetDB())
}

// ReloadAll refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *TwitterTweetSlice) ReloadAll(exec boil.Executor) error {
	if o == nil || len(*o) == 0 {
		return nil
	}

	twitterTweets := TwitterTweetSlice{}
	var args []interface{}
	for _, obj := range *o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), twitterTweetPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "SELECT \"twitter_tweets\".* FROM \"twitter_tweets\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, twitterTweetPrimaryKeyColumns, len(*o))

	q := queries.Raw(exec, sql, args...)

	err := q.Bind(&twitterTweets)
	if err != nil {
		return errors.Wrap(err, "mdbmodels: unable to reload all in TwitterTweetSlice")
	}

	*o = twitterTweets

	return nil
}

// TwitterTweetExists checks if the TwitterTweet row exists.
func TwitterTweetExists(exec boil.Executor, id int64) (bool, error) {
	var exists bool
	sql := "select exists(select 1 from \"twitter_tweets\" where \"id\"=$1 limit 1)"

	if boil.DebugMode {
		fmt.Fprintln(boil.DebugWriter, sql)
		fmt.Fprintln(boil.DebugWriter, id)
	}

	row := exec.QueryRow(sql, id)

	err := row.Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, "mdbmodels: unable to check if twitter_tweets exists")
	}

	return exists, nil
}

// TwitterTweetExistsG checks if the TwitterTweet row exists.
func TwitterTweetExistsG(id int64) (bool, error) {
	return TwitterTweetExists(boil.GetDB(), id)
}

// TwitterTweetExistsGP checks if the TwitterTweet row exists. Panics on error.
func TwitterTweetExistsGP(id int64) bool {
	e, err := TwitterTweetExists(boil.GetDB(), id)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// TwitterTweetExistsP checks if the TwitterTweet row exists. Panics on error.
func TwitterTweetExistsP(exec boil.Executor, id int64) bool {
	e, err := TwitterTweetExists(exec, id)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}
