package ormlite

import (
	"database/sql"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"strings"
)

func buildUpsertQuery(info *modelInfo) (string, []interface{}) {
	var (
		query                         = "insert into %s(%s) values(%s) on conflict(%s) do update set %s"
		fields, indexes, updateFields []string
		args                          []interface{}
	)
	for _, field := range info.fields {
		switch {
		case isReferenceField(field):
			if field.reference.Type != "has_one" {
				continue
			}
			fallthrough
		case isPkField(field):
			indexes = append(indexes, field.column)
			if isZeroField(field.value) {
				continue
			}
			fallthrough
		default:
			fields = append(fields, field.column)
			updateFields = append(updateFields, fmt.Sprintf("%s = ?", field.column))
		}
		args = append(args, field.value.Interface())
	}
	args = append(args, args...)
	return fmt.Sprintf(
		query, info.table, strings.Join(fields, ","),
		strings.Trim(strings.Repeat("?,", len(fields)), ","),
		strings.Join(indexes, ","), strings.Join(updateFields, ",")), args
}

func upsert(db *sql.DB, m IModel) error {
	mInfo, err := getModelInfo(m)
	if err != nil {
		return err
	}

	q, a := buildUpsertQuery(mInfo)
	result, err := db.Exec(q, a...)
	if err != nil {
		return &Error{err, q, a}
	}
	// check if there were last inserted id and apply it to primary key
	for _, field := range mInfo.fields {
		if isPkField(field) && !isReferenceField(field) {
			if isZeroField(field.value) {
				id, err := result.LastInsertId()
				if err != nil {
					return err
				}
				if field.value.Type().Kind() != reflect.Int64 {
					return errors.New("primary key is not int64")
				}
				field.value.SetInt(id)
			}
		}
	}
	return nil
}
