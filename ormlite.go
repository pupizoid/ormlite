package ormlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"
)

type relationType int

const (
	queryTimeout = time.Second * 30

	packageTagName       = "ormlite"
	defaultRelationDepth = 1

	noRelation relationType = 1 << iota
	hasMany
	hasOne
	manyToMany
)

var (
	// ErrNoRowsAffected is an error to return when no rows were affected
	ErrNoRowsAffected = errors.New("no rows affected")
)

// Error is a custom struct that contains sql error, query and arguments
type Error struct {
	SQLError error
	Query    string
	Args     []interface{}
}

// Error implements error interface
func (e *Error) Error() string { return e.SQLError.Error() }

// OrderBy describes ordering rule
type OrderBy struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

// Where is a map containing fields and their values to meet in the result
type Where map[string]interface{}

const (
	// AND is a glue between multiple statements after `where`
	AND = " and "
	// OR is a glue between multiple statements after `where`
	OR = " or "
)

// Options represents query options
type Options struct {
	Where         Where    `json:"where"`
	Divider       string   `json:"divider"`
	Limit         int      `json:"limit"`
	Offset        int      `json:"offset"`
	OrderBy       *OrderBy `json:"order_by"`
	RelationDepth int      `json:"relation_depth"`
	Related       []IModel `json:"related"`
}

// DefaultOptions returns default options for query
func DefaultOptions() *Options {
	return &Options{RelationDepth: defaultRelationDepth, Divider: AND}
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
	Table       string
	Type        relationType
	RelatedType reflect.Type
	FieldName   string
	Condition   string
	RefPkValue  interface{}
}

type columnInfo struct {
	RelationInfo relationInfo
	Name         string
	Index        int
}

func isExportedField(f reflect.StructField) bool {
	return strings.ToLower(string([]rune(f.Name)[0])) != string([]rune(f.Name)[0])
}

func lookForSettingWithSep(s, setting, sep string) string {
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		kvs := strings.SplitN(pair, sep, 2)
		if len(kvs) == 1 && kvs[0] == setting {
			return setting
		} else if len(kvs) == 2 && kvs[0] == setting {
			return kvs[1]
		}
	}
	return ""
}

func lookForSetting(s, setting string) string {
	return lookForSettingWithSep(s, setting, "=")
}

func getColumnInfo(t reflect.Type) ([]columnInfo, error) {

	var (
		columns []columnInfo
		v       = reflect.New(t)
	)

	for i := 0; i < t.NumField(); i++ {
		if !isExportedField(t.Field(i)) {
			continue
		}

		tag := t.Field(i).Tag.Get(packageTagName)
		if tag == "-" {
			continue
		}

		var ci = columnInfo{Index: i}
		if exp, ok := v.Elem().Field(i).Interface().(Expression); ok {
			ci.Name = exp.Column()
		} else {
			ci.Name = getFieldColumnName(t.Field(i))
		}

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
		info.RelatedType = field.Type
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
		info.RelatedType = field.Type.Elem()
		tOption := lookForSetting(t, "table")
		//if strings.Contains(tOption, "(") {
		//	info.Condition = tOption[strings.Index(tOption, "(")+1 : strings.Index(tOption, ")")]
		//	tOption = tOption[:strings.Index(tOption, "(")]
		//}
		info.Condition = lookForSettingWithSep(t, "condition", ":")
		info.Table = tOption
		info.FieldName = lookForSetting(t, "field")
	} else if strings.Contains(t, "has_many") {
		info.RelatedType = field.Type.Elem()
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
		if opts.Where != nil && len(opts.Where) != 0 {
			var keys []string
			for k, v := range opts.Where {
				switch reflect.TypeOf(v).Kind() {
				case reflect.Slice:
					if strings.Contains(k, ",") {
						rowValueCount := len(strings.Split(k, ","))
						for i := 0; i < len(v.([]interface{}))/rowValueCount; i++ {
							keys = append(keys, fmt.Sprintf("(%s) = (%s)", k, strings.Trim(strings.Repeat("?,", rowValueCount), ",")))
						}
						opts.Divider = OR
					} else {
						count := len(v.([]interface{}))
						if opts.Limit != 0 && opts.Limit < count {
							count = opts.Limit
						}
						keys = append(keys, fmt.Sprintf("%s in (%s)", k, strings.Trim(strings.Repeat("?,", count), ",")))
					}
					values = append(values, v.([]interface{})...)
				case reflect.String:
					keys = append(keys, fmt.Sprintf("%s like ?", k))
					values = append(values, fmt.Sprintf("%%%s%%", v))
				default:
					keys = append(keys, fmt.Sprintf("%s = ?", k))
					values = append(values, v)
				}
			}
			if len(keys) > 0 {
				q += fmt.Sprintf(" where %s", strings.Join(keys, opts.Divider))
			}
		}
		if opts.OrderBy != nil {
			q += fmt.Sprintf(" order by %s %s", opts.OrderBy.Field, opts.OrderBy.Order)
		}
		if opts.Limit != 0 {
			q += fmt.Sprintf(" limit %d", opts.Limit)
			if opts.Offset != 0 {
				q += fmt.Sprintf(" offset %d", opts.Offset)
			}
		}
	}
	rows, err := db.QueryContext(ctx, q, values...)
	if err != nil {
		return nil, &Error{err, q, values}
	}
	return rows, nil
}

func getPrimaryFieldsInfo(value reflect.Value) ([]pkFieldInfo, error) {
	var pkFields []pkFieldInfo
	for k := 0; k < value.NumField(); k++ {
		fv := value.Field(k)
		ft := value.Type().Field(k)
		if lookForSetting(ft.Tag.Get(packageTagName), "primary") == "primary" {
			var info pkFieldInfo
			info.name = getFieldColumnName(ft)
			info.field = fv
			info.relationName = lookForSetting(ft.Tag.Get(packageTagName), "ref")
			pkFields = append(pkFields, info)
		}
	}
	return pkFields, nil
}

func loadRelationsForSlice(ctx context.Context, db *sql.DB, opts *Options, slicePtr reflect.Value, colInfoPerEntry [][]columnInfo) error {
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
						pkFields, err := getPrimaryFieldsInfo(modelValue)
						if err != nil {
							return err
						}
						if err := loadHasManyRelation(ctx, db, ci.RelationInfo, modelValue.Field(ci.Index), pkFields, slicePtr.Index(i).Type(), opts); err != nil {
							return err
						}
					case manyToMany:
						pkFields, err := getPrimaryFieldsInfo(modelValue)
						if err != nil {
							return err
						}
						if err := loadManyToManyRelation(ctx, db, &ci.RelationInfo, modelValue.Field(ci.Index), pkFields, opts); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}

func loadStructRelations(ctx context.Context, db *sql.DB, opts *Options, out Model, pkField []pkFieldInfo, relations map[*relationInfo]reflect.Value) error {
	if opts == nil || opts.RelationDepth != 0 {
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
				if err := loadHasManyRelation(ctx, db, *ri, rv, pkField, reflect.TypeOf(out), opts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func loadHasManyRelation(ctx context.Context, db *sql.DB, ri relationInfo, fieldValue reflect.Value, pkFields []pkFieldInfo, parentType reflect.Type, options *Options) error {
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

	where := Where{}
	for i := 0; i < rve.NumField(); i++ {
		f := rve.Field(i)
		if f.Type.AssignableTo(parentType) {
			for _, pkf := range pkFields {
				where[getFieldColumnName(f)] = pkf.field.Interface()
			}
		}
	}

	if len(where) == 0 {
		return errors.New("failed to load has many relation since none fields of related type meet parent type")
	}

	return QuerySliceContext(ctx, db, WithWhere(&Options{RelationDepth: options.RelationDepth - 1, Limit: options.Limit, Divider: OR},
		where), fieldValue.Addr().Interface())
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

func loadManyToManyRelation(ctx context.Context, db *sql.DB, ri *relationInfo, rv reflect.Value, pkFields []pkFieldInfo, options *Options) error {
	var (
		refPkField, PkField, where []string
		args                       []interface{}
		relatedQueryConditions     = make(Where)
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
			refPkField = append(refPkField, lookForSetting(t, "ref"))
			PkField = append(PkField, getFieldColumnName(rve.Field(i)))
		}
	}

	if len(refPkField) < 1 {
		return errors.New("can't load relations: related struct does not have primary key")
	}

	for _, pkField := range pkFields {
		where = append(where, fmt.Sprintf("%s = ?", pkField.relationName))
		args = append(args, pkField.field.Interface())
	}
	if ri.Condition != "" {
		where = append(where, ri.Condition)
	}

	var whereClause string
	if len(pkFields) != 0 {
		whereClause = " where " + strings.Join(where, AND)
	}

	query := fmt.Sprintf("select %s from %s%s", strings.Join(refPkField, ","), ri.Table, whereClause)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return &Error{err, query, args}
	}

	for rows.Next() {
		var relatedPrimaryKeyValues []interface{}
		for i := 0; i < len(PkField); i++ {
			var relatedPk interface{}
			relatedPrimaryKeyValues = append(relatedPrimaryKeyValues, &relatedPk)
		}
		if err := rows.Scan(relatedPrimaryKeyValues...); err != nil {
			return err
		}
		if _, ok := relatedQueryConditions[strings.Join(PkField, ",")]; !ok {
			relatedQueryConditions[strings.Join(PkField, ",")] = relatedPrimaryKeyValues
		} else {
			relatedQueryConditions[strings.Join(PkField, ",")] = append(
				relatedQueryConditions[strings.Join(PkField, ",")].([]interface{}), relatedPrimaryKeyValues...)
		}
	}
	if len(relatedQueryConditions) == 0 {
		return nil // query has no rows so there is no need to load any model
	}
	return QuerySliceContext(
		ctx, db, WithWhere(&Options{
			RelationDepth: options.RelationDepth - 1, Divider: options.Divider, Limit: options.Limit},
			relatedQueryConditions),
		rv.Addr().Interface(),
	)
}

// QueryStruct looks up for rows in given table and scans it to provided struct or slice of structs
func QueryStruct(db *sql.DB, opts *Options, out Model) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	return QueryStructContext(ctx, db, opts, out)
}

// QueryStructContext looks up for rows in given table and scans it to provided struct or slice of structs
func QueryStructContext(ctx context.Context, db *sql.DB, opts *Options, out Model) error {
	model := reflect.ValueOf(out).Elem()
	if model.Type().Kind() != reflect.Struct {
		return fmt.Errorf("expected pointer to struct, got %T", model.Type())
	}

	var (
		pkFields  []pkFieldInfo
		columns   []string
		fieldPTRs []interface{}
		relations = make(map[*relationInfo]reflect.Value)
	)

	pkFields, err := getPrimaryFieldsInfo(model)
	if err != nil {
		return errors.Wrap(err, "failed to load struct")
	}

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
				fieldPTRs = append(fieldPTRs, &ri.RefPkValue)
			}
			relations[ri] = model.Field(i)
			continue
		}
		if exp, ok := model.Field(i).Interface().(Expression); ok {
			columns = append(columns, exp.Column())
		} else {
			columns = append(columns, getFieldColumnName(model.Type().Field(i)))
		}
		fieldPTRs = append(fieldPTRs, model.Field(i).Addr().Interface())
	}

	if len(columns) == 0 && len(relations) != 0 {
		goto Relations
	}

	{
		if opts != nil && len(opts.Related) != 0 {
			searchModels := map[reflect.Type][]Model{}
			for _, sm := range opts.Related {
				mt := reflect.TypeOf(sm)
				if slice, ok := searchModels[mt]; ok {
					slice = append(slice, sm)
				} else {
					searchModels[mt] = []Model{sm}
				}
			}
			for rInfo := range relations {
				spew.Dump(rInfo)
			}
		}
		rows, err := queryWithOptions(ctx, db, out.Table(), columns, opts)
		if err != nil {
			return err
		}

		for rows.Next() {
			if err := rows.Scan(fieldPTRs...); err != nil {
				return err
			}
		}
	}

Relations:
	return loadStructRelations(ctx, db, opts, out, pkFields, relations)
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

	var (
		modelType       = slicePtr.Type().Elem().Elem()
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
			fPtrs        []interface{}
			entryColInfo = make([]columnInfo, len(colInfo))
		)

		copy(entryColInfo, colInfo)
		colInfoPerEntry = append(colInfoPerEntry, entryColInfo)

		for i := 0; i < se.Elem().NumField(); i++ {
			for k, ci := range colInfo {
				if ci.Index == i {
					if ci.RelationInfo.Type == hasOne {
						pToPk := &entryColInfo[k].RelationInfo.RefPkValue
						fPtrs = append(fPtrs, pToPk)
					} else if ci.RelationInfo.Type == hasMany || ci.RelationInfo.Type == manyToMany {
						continue
					} else {
						fPtrs = append(fPtrs, se.Elem().Field(i).Addr().Interface())
					}
				}
			}
		}

		if err := rows.Scan(fPtrs...); err != nil {
			return err
		}

		slicePtr.Set(reflect.Append(slicePtr, se))
	}

	return loadRelationsForSlice(ctx, db, opts, slicePtr, colInfoPerEntry)
}

// Delete removes model object from database by it's primary key
func Delete(db *sql.DB, m Model) (sql.Result, error) {
	modelValue := reflect.ValueOf(m).Elem()

	var (
		where    []string
		args     []interface{}
		pkFields []pkFieldInfo
	)

	for i := 0; i < modelValue.NumField(); i++ {
		fv := modelValue.Field(i)
		ft := modelValue.Type().Field(i)
		if lookForSetting(ft.Tag.Get(packageTagName), "primary") == "primary" {
			var info pkFieldInfo
			info.name = getFieldColumnName(ft)
			info.field = fv
			pkFields = append(pkFields, info)
		}
	}

	if len(pkFields) == 0 {
		return nil, errors.New("delete failed: model does not have primary key")
	}

	for _, pkField := range pkFields {
		if reflect.Zero(pkField.field.Type()).Interface() == pkField.field.Interface() {
			return nil, errors.New("delete failed: model's primary key has zero value")
		}

		where = append(where, fmt.Sprintf("%s = ?", pkField.name))
		args = append(args, pkField.field.Interface())
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := fmt.Sprintf("delete from %s where %s", m.Table(), strings.Join(where, " and "))
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, &Error{err, query, args}
	}
	return res, err
}

type pkFieldInfo struct {
	relationName string
	name         string
	field        reflect.Value
}

func Count(db *sql.DB, m Model, opts *Options) (int64, error) {
	var (
		query   strings.Builder
		args    []interface{}
		divider string
		count   int64
	)

	query.WriteString("select count() from ")
	query.WriteString(m.Table())

	if opts != nil {
		if opts.Where != nil {
			query.WriteString(" where ")
			if len(opts.Where) > 1 && opts.Divider == "" {
				return 0, errors.New("empty divider with multiple conditions")
			}
			divider = opts.Divider
			for f, v := range opts.Where {
				switch reflect.TypeOf(v).Kind() {
				case reflect.Slice:
					if strings.Contains(f, ",") {
						rowValueCount := len(strings.Split(f, ","))
						for i := 0; i < len(v.([]interface{}))/rowValueCount; i++ {
							query.WriteString("(" + f + ") = (" + strings.Trim(strings.Repeat("?,", rowValueCount), ",") + ")" + divider)
						}
						opts.Divider = OR
					} else {
						count := len(v.([]interface{}))
						if opts.Limit != 0 && opts.Limit < count {
							count = opts.Limit
						}
						query.WriteString(f + " in (" + strings.Trim(strings.Repeat("?,", count), ",") + ")" + divider)
					}
					args = append(args, v.([]interface{})...)
				case reflect.String:
					query.WriteString(f + " like ?" + divider)
					args = append(args, fmt.Sprintf("%%%s%%", v))
				default:
					query.WriteString(f + " = ?" + divider)
					args = append(args, v)
				}
			}
		}
	}

	row := db.QueryRow(strings.TrimSuffix(query.String(), divider), args...)
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
