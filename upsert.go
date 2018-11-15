package ormlite

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

func sliceAsArray(s []interface{}) interface{} {
	arr := reflect.New(reflect.ArrayOf(len(s), reflect.TypeOf(s).Elem())).Elem()
	for i, j := range s {
		v := reflect.ValueOf(j)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		arr.Index(i).Set(v)
	}
	return arr.Interface()
}

func buildJoinQuery(info *modelInfo, field modelField) (string, []interface{}, error) {
	var (
		query          = "select %s from %s %s"
		where, columns []string
		args           []interface{}
		whereString    string
	)
	ri, err := getModelInfo(field.value)
	if err != nil {
		return "", nil, err
	}
	for _, f := range ri.fields {
		if isPkField(f) {
			columns = append(columns, field.reference.table+"."+f.reference.column)
		}
	}
	for _, f := range info.fields {
		if isPkField(f) {
			where = append(where,
				fmt.Sprintf("%s.%s = ?", field.reference.table, f.reference.column))
			args = append(args, f.value.Interface())
		}
	}
	if field.reference.condition != "" {
		where = append(where, field.reference.condition)
	}
	if len(where) > 0 {
		whereString = "where " + strings.Join(where, AND)
	}
	return fmt.Sprintf(
		query, strings.Join(columns, ","), field.reference.table, whereString), args, nil
}

func buildUpsertQuery(info *modelInfo) (string, []interface{}) {
	var (
		query        = "insert into %s(%s) values(%s) on conflict(%s) do update set %s"
		updateFields []string
		args         []interface{}
	)
	columns, indexes, args := getModelColumns(info.fields)
	for _, f := range columns {
		updateFields = append(updateFields, fmt.Sprintf("%s = ?", f))
	}
	args = append(args, args...)
	return fmt.Sprintf(
		query, info.table, strings.Join(columns, ","),
		strings.Trim(strings.Repeat("?,", len(columns)), ","),
		strings.Join(indexes, ","), strings.Join(updateFields, ",")), args
}

func buildInsertRelationQuery(field modelField, info *modelInfo, values []interface{}, columns []string) (string, []interface{}) {
	var (
		query = "insert into %s(%s) values (%s)"
	)

	cond, condValue := extractConditionValue(field.reference.condition)
	if cond != "" {
		columns = append(columns, cond)
		values = append(values, condValue)
	}

	for _, f := range info.fields {
		if isPkField(f) {
			columns = append(columns, f.reference.column)
			values = append(values, f.value.Interface())
		}
	}
	return fmt.Sprintf(query, field.reference.table, strings.Join(columns, ","),
		strings.Trim(strings.Repeat("?,", len(columns)), ",")), values
}

func buildDeleteRelationQuery(field modelField, info *modelInfo, keys interface{}, columns []string) (string, []interface{}) {
	var (
		args  []interface{}
		where []string
		query = "delete from %s where %s"
		kVal  = reflect.ValueOf(keys)
	)

	for _, col := range columns {
		where = append(where, fmt.Sprintf("%s = ?", col))
	}

	for i := 0; i < kVal.Len(); i++ {
		args = append(args, kVal.Index(i).Interface())
	}

	for _, f := range info.fields {
		if isPkField(f) {
			where = append(where, fmt.Sprintf("%s = ?", f.reference.column))
			args = append(args, f.value.Interface())
		}
	}

	cond, condValue := extractConditionValue(field.reference.condition)
	if cond != "" {
		where = append(where, fmt.Sprintf("%s = ?", cond))
		args = append(args, condValue)
	}
	return fmt.Sprintf(query, field.reference.table, strings.Join(where, AND)), args
}

func syncRelations(ctx context.Context, db *sql.DB, info *modelInfo) error {
	for _, field := range info.fields {
		if isManyToMany(field) {
			if err := syncManyToManyRelation(ctx, db, field, info); err != nil {
				return err
			}
		} else if isHasOne(field) {
			if err := syncHasOneRelation(ctx, db, field); err != nil {
				return err
			}
		} else if isHasMany(field) {
			if err := syncHasManyRelation(ctx, db, field, info); err != nil {
				return err
			}
		}
	}
	return nil
}

func getRelationMapping(value reflect.Value) ([][]interface{}, error) {
	var r [][]interface{}
	for i := 0; i < value.Len(); i++ {
		keys, err := getModelPkKeys(value.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		r = append(r, keys)
	}
	return r, nil
}

func getStoredRelations(ctx context.Context, db *sql.DB, field modelField, info *modelInfo) ([]string, map[interface{}]bool, error) {
	q, a, err := buildJoinQuery(info, field)
	if err != nil {
		return nil, nil, err
	}

	rows, err := db.QueryContext(ctx, q, a...)
	if err != nil {
		return nil, nil, &Error{err, q, a}
	}

	cols, err := rows.Columns()
	var result = map[interface{}]bool{}

	for rows.Next() {
		var keys []interface{}
		for i := 0; i < len(cols); i++ {
			keys = append(keys, new(interface{}))
		}
		if err := rows.Scan(keys...); err != nil {
			return nil, nil, err
		}
		result[sliceAsArray(keys)] = false
	}
	return cols, result, nil
}

func syncManyToManyRelation(ctx context.Context, db *sql.DB, field modelField, info *modelInfo) error {
	refValues, err := getRelationMapping(field.value)
	if err != nil {
		return err
	}

	refColumns, mapping, err := getStoredRelations(ctx, db, field, info)
	if err != nil {
		return err
	}
	// mark existing relations in mapping
	for _, keys := range refValues {
		if _, ok := mapping[sliceAsArray(keys)]; !ok {
			// missing relation we need to add it
			q, a := buildInsertRelationQuery(field, info, keys, refColumns)

			if res, err := db.ExecContext(ctx, q, a...); err != nil {
				return &Error{err, q, a}
			} else {
				if ra, err := res.RowsAffected(); err != nil || ra == 0 {
					return errors.New("insert query din't affect any row")
				}
			}
		}
		mapping[sliceAsArray(keys)] = true
	}
	for keys, exists := range mapping {
		if !exists {
			q, a := buildDeleteRelationQuery(field, info, keys, refColumns)
			if res, err := db.ExecContext(ctx, q, a...); err != nil {
				return &Error{err, q, a}
			} else {
				if ra, err := res.RowsAffected(); err != nil || ra == 0 {
					return errors.New("delete query din't affect any row")
				}
			}
		}
	}
	return nil
}

func syncHasOneRelation(ctx context.Context, db *sql.DB, field modelField) error {
	if !field.value.IsValid() || field.value.IsNil() {
		return nil
	}
	return upsert(ctx, db, field.value.Interface().(IModel))
}

func syncHasManyRelation(ctx context.Context, db *sql.DB, field modelField, model *modelInfo) error {
	if !field.value.IsValid() || field.value.IsNil() {
		return nil
	}
	if field.value.Type().Kind() != reflect.Slice {
		return errors.New("has many relation value should be slice containing models")
	}
	for i := 0; i < field.value.Len(); i++ {
		ri, err := getModelInfo(field.value.Index(i))
		if err != nil {
			return err
		}
		for _, f := range ri.fields {
			if model.value.Type().AssignableTo(f.value.Type()) {
				f.value.Set(model.value)
			}
		}
		if err := upsert(ctx, db, ri.value.Addr().Interface().(IModel)); err != nil {
			return err
		}
	}
	return nil
}

func upsert(ctx context.Context, db *sql.DB, m IModel) error {
	mInfo, err := getModelInfo(m)
	if err != nil {
		return err
	}

	q, a := buildUpsertQuery(mInfo)
	if len(a) > 0 {
		// we need to perform update query only for models that have fields
		result, err := db.ExecContext(ctx, q, a...)
		if err != nil {
			return &Error{err, q, a}
		}

		if err := setModelPk(mInfo, result); err != nil {
			return err
		}
	}

	return syncRelations(ctx, db, mInfo)
}
