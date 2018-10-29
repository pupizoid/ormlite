package ormlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cast"
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

// ErrNoRowsAffected is an error to return when no rows were affected
var ErrNoRowsAffected = errors.New("no rows affected")

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
	Table      string
	Type       relationType
	FieldName  string
	Condition  string
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
		kvs := strings.SplitN(pair, "=", 2)
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
		tOption := lookForSetting(t, "table")
		if strings.Contains(tOption, "(") {
			info.Condition = tOption[strings.Index(tOption, "(")+1 : strings.Index(tOption, ")")]
			tOption = tOption[:strings.Index(tOption, "(")]
		}
		info.Table = tOption
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
		if opts.Where != nil && len(opts.Where) != 0 {
			var keys []string
			for k, v := range opts.Where {
				if reflect.TypeOf(v).Kind() == reflect.Slice {
					if strings.Contains(k, ",") {
						rowValueCount := len(strings.Split(k, ","))
						for i := 0; i < len(v.([]interface{}))/rowValueCount; i++ {
							keys = append(keys, fmt.Sprintf("(%s) = (%s)", k, strings.Trim(strings.Repeat("?,", rowValueCount), ",")))
						}
						opts.Divider = OR
					} else {
						keys = append(keys, fmt.Sprintf("%s in (%s)", k, strings.Trim(strings.Repeat("?,", len(v.([]interface{}))), ",")))
					}
					values = append(values, v.([]interface{})...)
				} else {
					keys = append(keys, fmt.Sprintf("%s = ?", k))
					values = append(values, v)
				}
			}
			if len(keys) > 0 {
				q += fmt.Sprintf(" where %s", strings.Join(keys, opts.Divider))
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
						if err := loadHasManyRelation(ctx, db, modelValue.Field(ci.Index), pkFields, slicePtr.Index(i).Type(), opts); err != nil {
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
				if err := loadHasManyRelation(ctx, db, rv, pkField, reflect.TypeOf(out), opts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func loadHasManyRelation(ctx context.Context, db *sql.DB, fieldValue reflect.Value, pkFields []pkFieldInfo, parentType reflect.Type, options *Options) error {
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
		*relField): pkFields[0].field.Interface()}), fieldValue.Addr().Interface())
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

	if ri.FieldName != "" {
		for _, pkField := range pkFields {
			where = append(where, fmt.Sprintf("%s = ?", pkField.relationName))
			args = append(args, pkField.field.Interface())
		}
		if ri.Condition != "" {
			where = append(where, ri.Condition)
		}
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
		ctx, db, WithWhere(&Options{RelationDepth: options.RelationDepth - 1, Divider: options.Divider}, relatedQueryConditions),
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
		columns = append(columns, getFieldColumnName(model.Type().Field(i)))
		fieldPTRs = append(fieldPTRs, model.Field(i).Addr().Interface())
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

	return loadRelationsForSlice(ctx, db, opts, slicePtr, colInfoPerEntry)
}

// Delete removes model object from database by it's primary key
func Delete(db *sql.DB, m Model) error {
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
		return errors.New("delete failed: model does not have primary key")
	}

	for _, pkField := range pkFields {
		if reflect.Zero(pkField.field.Type()).Interface() == pkField.field.Interface() {
			return errors.New("delete failed: model's primary key has zero value")
		}

		where = append(where, fmt.Sprintf("%s = ?", pkField.name))
		args = append(args, pkField.field.Interface())
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	query := fmt.Sprintf("delete from %s where %s", m.Table(), strings.Join(where, " and "))
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return &Error{err, query, args}
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

// Upsert does the same think as UpsertContext with default background context
func Upsert(db *sql.DB, m Model) error {
	return UpsertContext(context.Background(), db, m)
}

// UpsertContext inserts object into table or updates it's values if it's not exist or updates it
func UpsertContext(ctx context.Context, db *sql.DB, m Model) error {
	modelValue, modelType, err := reflectModel(m)
	if err != nil {
		return err
	}

	var (
		pkInfo    []pkFieldInfo
		relations = make(map[*relationInfo]interface{})
	)

	fields, values, err := parseQueryEntries(modelType, modelValue, &pkInfo, relations)
	if err != nil {
		return err
	}

	if len(fields) == 0 {
		goto Relations
	}

	{
		if err := upsertModel(ctx, db, pkInfo, fields, values, m); err != nil {
			return err
		}
	}

Relations:
	return syncManyToManyRelations(ctx, db, relations, pkInfo)
}

func syncManyToManyRelations(ctx context.Context, db *sql.DB, relations map[*relationInfo]interface{}, pkFields []pkFieldInfo) error {
	for rel, value := range relations {
		refPkFieldNames, err := getRefPkFieldName(rel, value)
		if err != nil {
			return err
		}

		var (
			where             []string
			args              []interface{}
			existingRelations = make(map[interface{}]bool)
		)
		if rel.FieldName != "" {
			for _, i := range pkFields {
				where = append(where, fmt.Sprintf("%s = ?", i.relationName))
				args = append(args, i.field.Interface())
			}
			if rel.Condition != "" {
				where = append(where, rel.Condition)
			}
		}
		var whereClause string
		if len(where) != 0 {
			whereClause = "where " + strings.Join(where, " and ")
		}

		query := fmt.Sprintf(
			"select %s from %s %s", strings.Join(refPkFieldNames, ","), rel.Table, whereClause)
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return &Error{err, query, args}
		}
		for rows.Next() {
			var refPkKeys []interface{}
			for i := 0; i < len(refPkFieldNames); i++ {
				refPkKeys = append(refPkKeys, new(interface{}))
			}
			if err := rows.Scan(refPkKeys...); err != nil {
				return err
			}
			refPkArr := reflect.New(reflect.ArrayOf(len(refPkKeys), reflect.TypeOf(refPkKeys).Elem())).Elem()
			for i, j := range refPkKeys {
				refPkArr.Index(i).Set(reflect.ValueOf(j).Elem())
			}
			existingRelations[refPkArr.Interface()] = false
		}

		for k := 0; k < reflect.ValueOf(value).Len(); k++ {
			relatedModel := reflect.ValueOf(value).Index(k).Elem()
			var refPkFieldValues []interface{} // todo: find a way to use multiple pk as map key
			for i := 0; i < relatedModel.Type().NumField(); i++ {
				t, ok := relatedModel.Type().Field(i).Tag.Lookup(packageTagName)
				if !ok {
					continue
				}
				if lookForSetting(t, "primary") == "primary" {
					if v, ok := relatedModel.Field(i).Interface().(Model); ok {
						info, err := getPrimaryFieldsInfo(reflect.ValueOf(v).Elem())
						if err != nil {
							return errors.New("failed to get primary fields of related model")
						}
						for _, field := range info {
							refPkFieldValues = append(refPkFieldValues, field.field.Interface())
						}
					} else {
						refPkFieldValues = append(refPkFieldValues, relatedModel.Field(i).Interface())
					}
				}
			}
			refPkFieldValuesArr := reflect.New(reflect.ArrayOf(len(refPkFieldValues), reflect.TypeOf(refPkFieldValues).Elem())).Elem()
			for i, j := range refPkFieldValues {
				refPkFieldValuesArr.Index(i).Set(reflect.ValueOf(j))
			}
			if _, ok := existingRelations[refPkFieldValuesArr.Interface()]; !ok {
				if err := insertMissingRelation(ctx, db, refPkFieldValues, rel, refPkFieldNames, pkFields); err != nil {
					return err
				}
			}
			existingRelations[refPkFieldValuesArr.Interface()] = true
		}
		if err := deleteObsoleteRelations(ctx, db, existingRelations, refPkFieldNames, rel, pkFields); err != nil {
			return err
		}
	}
	return nil
}

func deleteObsoleteRelations(ctx context.Context, db *sql.DB, relMap map[interface{}]bool, refPkFields []string, rel *relationInfo, pkFields []pkFieldInfo) error {
	for refPK, exists := range relMap {
		if !exists {
			var values []interface{}
			for i := 0; i < reflect.ValueOf(refPK).Len(); i++ {
				values = append(values, reflect.ValueOf(refPK).Index(i).Interface())
			}
			var fields []string
			for _, refPkField := range refPkFields {
				fields = append(fields, fmt.Sprintf("%s = ?", refPkField))
			}
			if rel.FieldName != "" {
				for _, i := range pkFields {
					fields = append(fields, i.relationName)
					values = append(values, i.field.Interface())
				}
				if rel.Condition != "" {
					fields = append(fields, rel.Condition)
				}
			}

			query := fmt.Sprintf(
				"delete from %s where %s", rel.Table, strings.Join(fields, AND))
			res, err := db.ExecContext(ctx, query, values...)
			if err != nil {
				return &Error{err, query, values}
			}
			ra, err := res.RowsAffected()
			if err != nil || ra == 0 {
				return fmt.Errorf("failed to get rows affected of removed relations delete or it's 0 (%v)", err)
			}
		}
	}
	return nil
}

func insertMissingRelation(ctx context.Context, db *sql.DB, refPkKeys []interface{}, rel *relationInfo, refPkFields []string, pkFields []pkFieldInfo) error {
	values := refPkKeys
	fields := refPkFields
	if rel.Condition != "" { // todo: implement support of most conditional operators
		cond := strings.Split(rel.Condition, "=")
		if cond[0] != "" {
			fields = append(fields, cond[0])
			if cond[1] != "" {
				if strings.Contains(cond[1], "\"") {
					values = append(values, cast.ToString(cond[1]))
				} else {
					values = append(values, cast.ToInt64(cond[1]))
				}
			} else {
				return errors.New("conditional field does not have value, check field tag")
			}
		}

	}
	for _, i := range pkFields {
		fields = append(fields, i.relationName)
		values = append(values, i.field.Interface())
	}

	query := fmt.Sprintf(
		"insert into %s(%s) values(%s)", rel.Table, strings.Join(fields, ","), strings.Trim(
			strings.Repeat("?,", len(values)), ","))
	res, err := db.ExecContext(ctx, query, values...)
	if err != nil {
		return &Error{err, query, values}
	}
	ra, err := res.RowsAffected()
	if err != nil || ra == 0 {
		return fmt.Errorf("failed to get rows affected of missing relations add or it's 0 (%v)", err)
	}
	return nil
}

func getRefPkFieldName(rel *relationInfo, i interface{}) ([]string, error) {
	if rel.Table == "" {
		return nil, errors.New("failed to process relations: not enough settings")
	}
	rv := reflect.ValueOf(i)
	if rv.Kind() != reflect.Slice {
		return nil, errors.New("failed to process relations: wrong field type")
	}
	rvt := rv.Type().Elem()
	if rvt.Kind() != reflect.Ptr {
		return nil, errors.New("failed to process relations: wrong field type")
	}
	rvs := rvt.Elem()
	if rvs.Kind() != reflect.Struct {
		return nil, errors.New("failed to process relations: wrong field type")
	}
	var refFieldName []string
	for i := 0; i < rvs.NumField(); i++ {
		tag, ok := rvs.Field(i).Tag.Lookup(packageTagName)
		if !ok {
			continue
		}
		if lookForSetting(tag, "primary") == "primary" {
			field := lookForSetting(tag, "ref")
			if field != "" {
				refFieldName = append(refFieldName, field)
			}
		}
	}
	if len(refFieldName) == 0 {
		return nil, errors.New("related type does not have primary key or reference field name")
	}
	return refFieldName, nil
}

type pkFieldInfo struct {
	relationName string
	name         string
	field        reflect.Value
}

func upsertModel(ctx context.Context, db *sql.DB, info []pkFieldInfo, fields []string, values []interface{}, m Model) error {
	var (
		query      string
		fieldPairs []string
		indexes    []string
	)
	for _, f := range fields {
		fieldPairs = append(fieldPairs, fmt.Sprintf("%s = ?", f))
	}
	for _, fi := range info {
		if reflect.Zero(fi.field.Type()).Interface() != fi.field.Interface() {
			fields = append(fields, fi.name)
			values = append(values, fi.field.Interface())
		}
		indexes = append(indexes, fi.name)
	}
	values = append(values, values...)
	query = fmt.Sprintf(
		fmt.Sprintf("insert into %s(%s) values(%s) on conflict(%s) do update set %s",
			m.Table(), strings.Join(fields, ","), strings.Trim(strings.Repeat("?,", len(fields)), ","),
			strings.Join(indexes, ","), strings.Join(fieldPairs, ",")),
	)
	res, err := db.ExecContext(ctx, query, values...)
	if err != nil {
		return &Error{err, query, values}
	}
	ra, err := res.RowsAffected()
	if err != nil || ra == 0 {
		return errors.New("no rows were affected")
	}
	// if it was insert query - set new id to entry
	if reflect.Zero(info[0].field.Type()).Interface() == info[0].field.Interface() {
		iid, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last inserted id: %v", err)
		}
		if info[0].field.Kind() != reflect.Int64 {
			return errors.New("insert functionality can be used only for models with int64 primary keys")
		}
		info[0].field.SetInt(iid)
	}
	return nil
}

func parseQueryEntries(modelType reflect.Type, value reflect.Value, pkFields *[]pkFieldInfo, relations map[*relationInfo]interface{}) ([]string, []interface{}, error) {
	var (
		fields []string
		values []interface{}
	)

	for i := 0; i < modelType.NumField(); i++ {
		if !isExportedField(modelType.Field(i)) {
			continue
		}

		fTag := modelType.Field(i).Tag.Get(packageTagName)
		if fTag == "-" {
			continue
		}

		if strings.Contains(fTag, "primary") {
			var info pkFieldInfo
			info.name = getFieldColumnName(modelType.Field(i))
			info.relationName = lookForSetting(fTag, "ref")
			info.field = value.Elem().Field(i)
			*pkFields = append(*pkFields, info)
			continue
		}

		if rInfo := extractRelationInfo(modelType.Field(i)); rInfo != nil {
			switch rInfo.Type {
			case hasOne:
				refValue := reflect.ValueOf(value.Elem().Field(i).Interface())
				if refValue.Kind() != reflect.Ptr {
					return nil, nil, fmt.Errorf("one-to-one mtmRelations supports only pointer to struct, not %T", value.Elem().Field(i).Interface())
				}
				var refPkFieldValue interface{}
				for i := 0; i < refValue.Type().Elem().NumField(); i++ {
					if lookForSetting(refValue.Type().Elem().Field(i).Tag.Get(packageTagName), "primary") == "primary" {
						if refValue.IsValid() && refValue.Elem().IsValid() {
							refPkFieldValue = refValue.Elem().Field(i).Interface()
						}
					}
				}
				pkField := getFieldColumnName(modelType.Field(i))
				if pkField == "" {
					return nil, nil, errors.New("one-to-one related struct don't have primary key")
				}
				values = append(values, refPkFieldValue)
				fields = append(fields, pkField)
			case manyToMany:
				relations[rInfo] = value.Elem().Field(i).Interface()
			}
			continue
		}

		fields = append(fields, getFieldColumnName(modelType.Field(i)))
		values = append(values, value.Elem().Field(i).Interface())
	}
	return fields, values, nil
}

func reflectModel(m Model) (reflect.Value, reflect.Type, error) {
	ev := reflect.ValueOf(m)
	if ev.Kind() != reflect.Ptr {
		return reflect.Value{}, nil, fmt.Errorf("model expected to be ptr, %v given", ev.Kind())
	}
	spew.Dump(ev.CanSet())
	et := ev.Elem().Type()
	if et.Kind() != reflect.Struct {
		return reflect.Value{}, nil, fmt.Errorf("model expected to be a pointer to a struct, not to %v", et.Kind())
	}
	return ev, et, nil
}
