package ormlite

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type relationType int

const (
	queryTimeout = time.Second * 5

	packageTagName       = "ormlite"
	defaultRelationDepth = 1

	noRelation relationType = 1 << iota
	hasMany
	hasOne
	manyToMany
)

var ErrNoRowsAffected = errors.New("no rows affected")

// OrderBy describes ordering rule
type OrderBy struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type Where map[string]interface{}

// Options represents query options
type Options struct {
	Where         Where    `json:"where"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
	OrderBy       *OrderBy `json:"order_by"`
	RelationDepth int      `json:"relation_depth"`
}

// DefaultOptions returns default options for query
func DefaultOptions() *Options {
	return &Options{RelationDepth: defaultRelationDepth}
}

// WithWhere modifies existing options by adding where clause to them
func WithWhere(options *Options, where Where) *Options {
	options.Where = where
	return options
}

// WithLimit modifies existing options by adding limit parameter to them
func WithLimit(options *Options, limit int) *Options {
	options.Limit = limit
	return options
}

// WithOffset modifies existing options by adding offset parameter to them.
// If options does not have positive limit parameter the offset will remain unchanged
// to avoid sql query correctness.
func WithOffset(options *Options, offset int) *Options {
	if options.Limit != 0 {
		options.Offset = offset
	}
	return options
}

// WithOrder modifies existing options by adding ordering options to them
func WithOrder(options *Options, by OrderBy) *Options {
	options.OrderBy = &by
	return options
}

// Model is an interface that represents model of database
type Model interface {
	Table() string
}

type relationInfo struct {
	Table      string
	Type       relationType
	FieldName  string
	RefPkValue interface{}
}

type columnInfo struct {
	RelationInfo relationInfo
	Name         string
	Index        int
}

func isExportedField(f reflect.StructField) bool {
	return strings.ToLower(string([]rune(f.Name)[0])) != string([]rune(f.Name)[0])
}

func lookForSetting(s, setting string) string {
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		kvs := strings.Split(pair, "=")
		if len(kvs) == 1 && kvs[0] == setting {
			return setting
		} else if len(kvs) == 2 && kvs[0] == setting {
			return kvs[1]
		}
	}
	return ""
}

func getColumnInfo(t reflect.Type) ([]columnInfo, error) {
	var columns []columnInfo

	for i := 0; i < t.NumField(); i++ {
		if !isExportedField(t.Field(i)) {
			continue
		}

		tag := t.Field(i).Tag.Get(packageTagName)
		if tag == "-" {
			continue
		}

		var ci = columnInfo{Index: i, Name: getFieldColumnName(t.Field(i))}

		if ri := extractRelationInfo(t.Field(i)); ri != nil {
			ci.RelationInfo = *ri
		} else {
			ci.RelationInfo = relationInfo{Type: noRelation}
		}

		columns = append(columns, ci)
	}
	return columns, nil
}

func extractRelationInfo(field reflect.StructField) *relationInfo {
	var info = relationInfo{Type: noRelation}

	t, ok := field.Tag.Lookup(packageTagName)
	if !ok {
		return nil
	}

	if strings.Contains(t, "has_one") {
		info.Type = hasOne
		info.FieldName = getFieldColumnName(field)

		for i := 0; i < field.Type.Elem().NumField(); i++ {
			if lookForSetting(field.Type.Elem().Field(i).Tag.Get(packageTagName), "primary") == "primary" {
				info.RefPkValue = reflect.New(field.Type.Elem().Field(i).Type).Elem().Interface()
			}
		}
		if info.RefPkValue == nil {
			return nil // maybe we need to return an error here
		}
	} else if strings.Contains(t, "many_to_many") {
		info.Type = manyToMany
		info.Table = lookForSetting(t, "table")
		info.FieldName = lookForSetting(t, "field")
	} else if strings.Contains(t, "has_many") {
		info.Type = hasMany
	} else {
		return nil
	}
	return &info
}

func queryWithOptions(ctx context.Context, db *sql.DB, table string, columns []string, opts *Options) (*sql.Rows, error) {
	var values []interface{}
	q := fmt.Sprintf("select %s from %s", strings.Join(columns, ","), table)
	if opts != nil {
		if opts.Where != nil {
			var keys []string
			for k, v := range opts.Where {
				if reflect.TypeOf(v).Kind() == reflect.Slice {
					keys = append(keys, fmt.Sprintf("%s in (%s)", k, strings.Trim(strings.Repeat("?,", len(v.([]interface{}))), ",")))
					values = append(values, v.([]interface{})...)
				} else {
					keys = append(keys, fmt.Sprintf("%s = ?", k))
					values = append(values, v)
				}
			}
			if len(keys) > 0 {
				q += fmt.Sprintf(" where %s", strings.Join(keys, " AND "))
			}
		}
		if opts.Limit != 0 {
			q += fmt.Sprintf(" limit %d", opts.Limit)
			if opts.Offset != 0 {
				q += fmt.Sprintf(" offset %d", opts.Offset)
			}
		}
		if opts.OrderBy != nil {
			q += fmt.Sprintf(" order by %s %s", opts.OrderBy.Field, opts.OrderBy.Order)
		}
	}
	return db.QueryContext(ctx, q, values...)
}

func getFieldColumnName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup(packageTagName)
	if !ok || tag == "" {
		return strings.ToLower(field.Name)
	}
	pairs := strings.Split(tag, ",")
	for _, pair := range pairs {
		if strings.Contains(pair, "col") {
			kv := strings.Split(pair, "=")
			if len(kv) != 2 {
				return ""
			}
			return kv[1]
		}
	}
	return strings.ToLower(field.Name)
}

func getPrimaryFieldValue(value reflect.Value) (reflect.Value, error) {
	for k := 0; k < value.NumField(); k++ {
		if lookForSetting(value.Type().Field(k).Tag.Get(packageTagName), "primary") == "primary" {
			return value.Field(k), nil
		}
	}
	return reflect.Value{}, fmt.Errorf("related model does not have primary key field")
}

func loadHasManyRelation(ctx context.Context, db *sql.DB, fieldValue, pkField reflect.Value, parentType reflect.Type, options *Options) error {
	if fieldValue.Kind() != reflect.Slice {
		return fmt.Errorf("can't load relations: wrong field type: %v", fieldValue.Type())
	}
	rvt := fieldValue.Type().Elem()
	if rvt.Kind() != reflect.Ptr {
		return fmt.Errorf("can't load relations: wrong field type: %v", rvt)
	}
	rve := rvt.Elem()
	if rve.Kind() != reflect.Struct {
		return fmt.Errorf("can't load relations: wrong field type: %v", rve)
	}

	var relField *reflect.StructField
	for i := 0; i < rve.NumField(); i++ {
		f := rve.Field(i)
		if f.Type.AssignableTo(parentType) {
			relField = &f
		}
	}
	if relField == nil {
		return errors.New("failed to load has many relation since none fields of related type meet parent type")
	}
	return QuerySliceContext(ctx, db, WithWhere(&Options{RelationDepth: options.RelationDepth - 1}, Where{getFieldColumnName(
		*relField): pkField.Interface()}), fieldValue.Addr().Interface())
}

func loadHasOneRelation(ctx context.Context, db *sql.DB, ri *relationInfo, rv reflect.Value, options *Options) error {
	if ri.RefPkValue == nil {
		return nil
	}

	_, ok := rv.Interface().(Model)
	if !ok {
		return fmt.Errorf("incorrect field value of one_to_one relation, expected ormlite.Model")
	}

	refObj := reflect.New(rv.Type().Elem())

	var refPkField string
	for i := 0; i < rv.Type().Elem().NumField(); i++ {
		tag := rv.Type().Elem().Field(i).Tag.Get(packageTagName)
		if lookForSetting(tag, "primary") == "primary" {
			refPkField = getFieldColumnName(rv.Type().Elem().Field(i))
		}
	}
	if refPkField == "" {
		return errors.New("referenced model does not have primary key")
	}
	if err := QueryStructContext(ctx, db, WithWhere(&Options{
		RelationDepth: options.RelationDepth - 1,
	}, Where{refPkField: ri.RefPkValue}), refObj.Interface().(Model)); err != nil {
		return err
	}
	rv.Set(refObj)
	return nil
}

func loadManyToManyRelation(ctx context.Context, db *sql.DB, ri *relationInfo, rv, pkField reflect.Value, options *Options) error {
	var (
		rPKField, PKField string
		rPKs              []interface{}
	)
	if rv.Kind() != reflect.Slice {
		return fmt.Errorf("can't load relations: wrong field type: %v", rv.Type())
	}
	rvt := rv.Type().Elem()
	if rvt.Kind() != reflect.Ptr {
		return fmt.Errorf("can't load relations: wrong field type: %v", rvt)
	}
	rve := rvt.Elem()
	if rve.Kind() != reflect.Struct {
		return fmt.Errorf("can't load relations: wrong field type: %v", rve)
	}
	for i := 0; i < rve.NumField(); i++ {
		t, ok := rve.Field(i).Tag.Lookup(packageTagName)
		if !ok {
			continue
		}
		if lookForSetting(t, "primary") == "primary" {
			rPKField = lookForSetting(t, "ref")
			PKField = lookForSetting(t, "col")
			break
		}
	}

	var (
		where string
		args  []interface{}
	)
	if ri.FieldName != "" {
		where = fmt.Sprintf("where %s = ?", ri.FieldName)
		args = append(args, pkField.Interface())
	}
	rows, err := db.QueryContext(ctx, fmt.Sprintf("select %s from %s %s", rPKField, ri.Table, where), args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var rPK int
		if err := rows.Scan(&rPK); err != nil {
			return err
		}
		rPKs = append(rPKs, rPK)
	}
	return QuerySliceContext(
		ctx, db, WithWhere(&Options{RelationDepth: options.RelationDepth - 1}, Where{PKField: rPKs}),
		rv.Addr().Interface(),
	)
}

// QueryStruct looks up for rows in given table and scans it to provided struct or slice of structs
func QueryStruct(db *sql.DB, opts *Options, out Model) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	return QueryStructContext(ctx, db, opts, out)
}

// QueryStruct looks up for rows in given table and scans it to provided struct or slice of structs
func QueryStructContext(ctx context.Context, db *sql.DB, opts *Options, out Model) error {
	model := reflect.ValueOf(out).Elem()
	if model.Type().Kind() != reflect.Struct {
		return fmt.Errorf("expected pointer to struct, got %T", model.Type())
	}

	var (
		pkField   reflect.Value
		columns   []string
		fieldPtrs []interface{}
		relations = make(map[*relationInfo]reflect.Value)
	)

	for i := 0; i < model.NumField(); i++ {

		if !isExportedField(model.Type().Field(i)) {
			continue
		}

		tag := model.Type().Field(i).Tag.Get(packageTagName)
		if tag == "-" {
			continue
		}

		if ri := extractRelationInfo(model.Type().Field(i)); ri != nil {
			if ri.Type == hasOne {
				columns = append(columns, getFieldColumnName(model.Type().Field(i)))
				fieldPtrs = append(fieldPtrs, &ri.RefPkValue)
			}
			relations[ri] = model.Field(i)
			continue
		}
		columns = append(columns, getFieldColumnName(model.Type().Field(i)))
		fieldPtrs = append(fieldPtrs, model.Field(i).Addr().Interface())

		if lookForSetting(tag, "primary") == "primary" {
			pkField = model.Field(i)
		}
	}

	if len(columns) == 0 && len(relations) != 0 {
		goto Relations
	}

	{
		rows, err := queryWithOptions(ctx, db, out.Table(), columns, opts)
		if err != nil {
			return err
		}

		for rows.Next() {
			if err := rows.Scan(fieldPtrs...); err != nil {
				return err
			}
		}
		if opts == nil || opts.RelationDepth < 1 {
			return nil
		}
	}

Relations:
	// load relations
	for ri, rv := range relations {
		if ri.Type == manyToMany {
			if err := loadManyToManyRelation(ctx, db, ri, rv, pkField, opts); err != nil {
				return err
			}
		} else if ri.Type == hasOne {
			if err := loadHasOneRelation(ctx, db, ri, rv, opts); err != nil {
				return err
			}
		} else if ri.Type == hasMany {
			if err := loadHasManyRelation(ctx, db, rv, pkField, reflect.TypeOf(out), opts); err != nil {
				return err
			}
		}
	}
	return nil
}

// QuerySlice scans rows into the slice of structs
func QuerySlice(db *sql.DB, opts *Options, out interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	return QuerySliceContext(ctx, db, opts, out)
}

// QuerySliceContext scans rows into the slice of structs with given context
func QuerySliceContext(ctx context.Context, db *sql.DB, opts *Options, out interface{}) error {

	slicePtr := reflect.ValueOf(out).Elem()
	if !slicePtr.Type().Elem().Implements(reflect.TypeOf((*Model)(nil)).Elem()) {
		return errors.New("slice contain type that does not implement Model interface")
	}

	modelType := slicePtr.Type().Elem().Elem() //

	var (
		colNames        []string
		colInfoPerEntry [][]columnInfo
	)

	colInfo, err := getColumnInfo(modelType)
	if err != nil {
		return fmt.Errorf("failed to get column info for type: %v", modelType)
	}

	for _, ci := range colInfo {
		if ci.RelationInfo.Type == noRelation || ci.RelationInfo.Type == hasOne {
			colNames = append(colNames, ci.Name)
		}
	}

	rows, err := queryWithOptions(
		ctx, db, reflect.New(modelType).Interface().(Model).Table(), colNames, opts)
	if err != nil {
		return err
	}

	for rows.Next() {

		var (
			se           = reflect.New(modelType)
			fptrs        []interface{}
			entryColInfo = make([]columnInfo, len(colInfo))
		)

		copy(entryColInfo, colInfo)
		colInfoPerEntry = append(colInfoPerEntry, entryColInfo)

		for i := 0; i < se.Elem().NumField(); i++ {
			for k, ci := range colInfo {
				if ci.Index == i {
					if ci.RelationInfo.Type == hasOne {
						pToPk := &entryColInfo[k].RelationInfo.RefPkValue
						fptrs = append(fptrs, pToPk)
					} else if ci.RelationInfo.Type == hasMany || ci.RelationInfo.Type == manyToMany {
						continue
					} else {
						fptrs = append(fptrs, se.Elem().Field(i).Addr().Interface())
					}
				}
			}
		}

		if err := rows.Scan(fptrs...); err != nil {
			return err
		}

		slicePtr.Set(reflect.Append(slicePtr, se))
	}

	if opts != nil && opts.RelationDepth != 0 {
		for i := 0; i < slicePtr.Len(); i++ {
			for _, ci := range colInfoPerEntry[i] {
				if ci.RelationInfo.Type != noRelation {
					var modelValue = slicePtr.Index(i).Elem()

					switch ci.RelationInfo.Type {
					case hasOne:
						if err := loadHasOneRelation(ctx, db, &ci.RelationInfo, modelValue.Field(ci.Index), opts); err != nil {
							return err
						}
					case hasMany:
						pkField, err := getPrimaryFieldValue(modelValue)
						if err != nil {
							return err
						}
						if err := loadHasManyRelation(ctx, db, modelValue.Field(ci.Index), pkField, slicePtr.Index(i).Type(), opts); err != nil {
							return err
						}
					case manyToMany:
						pkField, err := getPrimaryFieldValue(modelValue)
						if err != nil {
							return err
						}
						if err := loadManyToManyRelation(ctx, db, &ci.RelationInfo, modelValue.Field(ci.Index), pkField, opts); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

// Delete removes model object from database by it's primary key
func Delete(db *sql.DB, m Model) error {
	modelValue := reflect.ValueOf(m).Elem()

	var pkFieldColumn string
	var pkField reflect.Value

	for i := 0; i < modelValue.NumField(); i++ {
		if lookForSetting(modelValue.Type().Field(i).Tag.Get(packageTagName), "primary") == "primary" {
			pkField = modelValue.Field(i)
			pkFieldColumn = getFieldColumnName(modelValue.Type().Field(i))
		}
	}

	if !pkField.IsValid() {
		return errors.New("delete failed: model does not have primary key")
	}

	if reflect.Zero(pkField.Type()).Interface() == pkField.Interface() {
		return errors.New("delete failed: model's primary key has zero value")
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := fmt.Sprintf("delete from %s where %s = ?", m.Table(), pkFieldColumn)
	res, err := db.ExecContext(ctx, query, pkField.Interface())
	if err != nil {
		return err
	}

	ra, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "delete failed")
	}
	if ra == 0 {
		return ErrNoRowsAffected
	}

	return nil
}

func Upsert(db *sql.DB, m Model) error {
	return UpsertContext(context.Background(), db, m)
}

// Upsert inserts object into table or updates it's values if it's not exist or updates it
func UpsertContext(ctx context.Context, db *sql.DB, m Model) error {
	ev := reflect.ValueOf(m)
	if ev.Kind() != reflect.Ptr {
		return fmt.Errorf("model expected to be ptr, %v given", ev.Kind())
	}

	et := ev.Elem().Type()
	if et.Kind() != reflect.Struct {
		return fmt.Errorf("model expected to be a pointer to a struct, not to %v", et.Kind())
	}

	var (
		pk          interface{}
		pkField     reflect.Value
		pkFieldName string
		fields      []string
		values      []interface{}
		relations   = make(map[*relationInfo]interface{})
	)

	for i := 0; i < et.NumField(); i++ {
		if !isExportedField(et.Field(i)) {
			continue
		}

		fTag, ok := et.Field(i).Tag.Lookup(packageTagName)
		if !ok {
			fields = append(fields, strings.ToLower(et.Field(i).Name))
			values = append(values, ev.Elem().Field(i).Interface())
			continue
		}

		if fTag == "-" {
			continue
		}

		if strings.Contains(fTag, "primary") {
			if reflect.Zero(et.Field(i).Type).Interface() != ev.Elem().Field(i).Interface() {
				pk = ev.Elem().Field(i).Interface()
			}
			pkFieldName = getFieldColumnName(et.Field(i))
			pkField = ev.Elem().Field(i)
			continue
		}

		if rInfo := extractRelationInfo(et.Field(i)); rInfo != nil {
			switch rInfo.Type {
			case hasOne:
				refValue := reflect.ValueOf(ev.Elem().Field(i).Interface())
				if refValue.Kind() != reflect.Ptr {
					return fmt.Errorf("one-to-one relations supports only pointer to struct, not %T", ev.Elem().Field(i).Interface())
				}
				var refPkFieldValue interface{}
				for i := 0; i < refValue.Type().Elem().NumField(); i++ {
					if lookForSetting(refValue.Type().Elem().Field(i).Tag.Get(packageTagName), "primary") == "primary" {
						if refValue.IsValid() && refValue.Elem().IsValid() {
							refPkFieldValue = refValue.Elem().Field(i).Interface()
						}
					}
				}
				pkField := getFieldColumnName(et.Field(i))
				if pkField == "" {
					return errors.New("one-to-one related struct don't have primary key")
				}
				values = append(values, refPkFieldValue)
				fields = append(fields, pkField)
			case manyToMany:
				relations[rInfo] = ev.Elem().Field(i).Interface()
			}
			continue
		}

		fields = append(fields, getFieldColumnName(et.Field(i)))
		values = append(values, ev.Elem().Field(i).Interface())
	}

	if len(fields) == 0 && len(relations) != 0 {
		goto Relations
	}

	{
		var query string
		if pk == nil {
			query = fmt.Sprintf(
				"insert into %s(%s) values(%s)", m.Table(), strings.Join(fields, ","),
				strings.Trim(strings.Repeat("?,", len(fields)), ","),
			)
		} else {
			var fieldPairs []string
			for _, f := range fields {
				fieldPairs = append(fieldPairs, fmt.Sprintf("%s = ?", f))
			}
			values = append(values, pk)
			query = fmt.Sprintf(
				fmt.Sprintf("update %s set %s where %s = ?", m.Table(), strings.Join(fieldPairs, ","), pkFieldName),
			)
		}
		res, err := db.ExecContext(ctx, query, values...)
		if err != nil {
			return err
		}
		ra, err := res.RowsAffected()
		if err != nil || ra == 0 {
			return errors.New("no rows were affected")
		}
		// if it was insert query - set new id to entry
		if pk == nil {
			iid, err := res.LastInsertId()
			if err != nil {
				return fmt.Errorf("failed to get last inserted id: %v", err)
			}
			if pkField.Kind() != reflect.Int {
				return errors.New("insert functionality can be used only for models with int primary keys")
			}
			pkField.SetInt(iid)
			pk = iid
		}
	}

Relations:
	// if there were mtm relations process them
	for rel, iface := range relations {
		if rel.Table == "" {
			return errors.New("failed to process relations: not enough settings")
		}
		rv := reflect.ValueOf(iface)
		if rv.Kind() != reflect.Slice {
			return errors.New("failed to process relations: wrong field type")
		}
		rvt := rv.Type().Elem()
		if rvt.Kind() != reflect.Ptr {
			return errors.New("failed to process relations: wrong field type")
		}
		rvs := rvt.Elem()
		if rvs.Kind() != reflect.Struct {
			return errors.New("failed to process relations: wrong field type")
		}
		var refFieldName string
		for i := 0; i < rvs.NumField(); i++ {
			tag, ok := rvs.Field(i).Tag.Lookup(packageTagName)
			if !ok {
				continue
			}
			if lookForSetting(tag, "primary") == "primary" {
				refFieldName = lookForSetting(tag, "ref")
				break
			}
		}
		if refFieldName == "" {
			return errors.New("related type does not have primary key or reference field name")
		}

		var (
			where   string
			args    []interface{}
			exgRels = make(map[interface{}]bool)
		)
		if rel.FieldName != "" {
			where = fmt.Sprintf("where %s = ?", rel.FieldName)
			args = append(args, pkField.Interface())
		}

		rows, err := db.QueryContext(ctx, fmt.Sprintf("select %s from %s %s", refFieldName, rel.Table, where), args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var refPK int // TODO: we need some casting to support not only int PK's
			if err := rows.Scan(&refPK); err != nil {
				return err
			}
			exgRels[refPK] = false
		}
		for k := 0; k < reflect.ValueOf(iface).Len(); k++ {
			is := reflect.ValueOf(iface).Index(k).Elem()
			for i := 0; i < is.Type().NumField(); i++ {
				t, ok := is.Type().Field(i).Tag.Lookup(packageTagName)
				if !ok {
					continue
				}
				if lookForSetting(t, "primary") == "primary" {
					if _, ok := exgRels[is.Field(i).Interface()]; !ok {
						values := []interface{}{is.Field(i).Interface()}
						fields := fmt.Sprintf("%s", refFieldName)
						if rel.FieldName != "" {
							fields = fmt.Sprintf("%s, %s", rel.FieldName, refFieldName)
							values = append([]interface{}{pk}, values...)
						}
						res, err := db.ExecContext(ctx,
							fmt.Sprintf(
								"insert into %s(%s) values(%s)", rel.Table, fields, strings.Trim(strings.Repeat("?,", len(values)), ",")), values...)
						if err != nil {
							return err
						}
						ra, err := res.RowsAffected()
						if err != nil || ra == 0 {
							return fmt.Errorf("failed to get rows affected of missing relations add or it's 0 (%v)", err)
						}
					}
					exgRels[is.Field(i).Interface()] = true
				}
			}
		}
		// delete
		for refPK, exists := range exgRels {
			if !exists {
				values := []interface{}{refPK}
				fields := fmt.Sprintf("%s = ?", refFieldName)
				if rel.FieldName != "" {
					fields += fmt.Sprintf(" and %s = ?", rel.FieldName)
					values = append(values, pk)
				}
				res, err := db.ExecContext(ctx,
					fmt.Sprintf(
						"delete from %s where %s", rel.Table, fields), values...)
				if err != nil {
					return err
				}
				ra, err := res.RowsAffected()
				if err != nil || ra == 0 {
					return fmt.Errorf("failed to get rows affected of removed relations delete or it's 0 (%v)", err)
				}
			}
		}
	}

	return nil
}
