package ormlite

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type relationType int

const (
	packageTagName = "ormlite"

	noRelation relationType = 1 << iota
	oneToMany
	oneToOne
	manyToMany
)

// OrderBy describes ordering rule
type OrderBy struct {
	Field string
	Order string
}

// Options represents query options
type Options struct {
	Where         map[string]interface{}
	Limit         int
	Offset        int
	OrderBy       *OrderBy
	LoadRelations bool
}

// Model is an interface that represents model of database
type Model interface {
	Table() string
}

type relationInfo struct {
	Table     string
	Type      relationType
	FieldName string
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

func extractRelationInfo(field reflect.StructField) *relationInfo {
	var ri = relationInfo{Type: noRelation}

	t, ok := field.Tag.Lookup(packageTagName)
	if !ok {
		return nil
	}

	if strings.Contains(t, "one_to_one") {
		ri.Type = oneToOne
		if c := getFieldColumnName(field); c != "" {
			ri.FieldName = c
		} else {
			ri.FieldName = strings.ToLower(field.Name)
		}
	} else if strings.Contains(t, "many_to_many") {
		ri.Type = manyToMany
		ri.Table = lookForSetting(t, "table")
		ri.FieldName = lookForSetting(t, "field")
	} else {
		return nil
	}
	return &ri
}

func queryWithOptions(db *sql.DB, table string, columns []string, opts *Options) (*sql.Rows, error) {
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
		}
		if opts.Offset != 0 {
			q += fmt.Sprintf(" offset %d", opts.Offset)
		}
		if opts.OrderBy != nil {
			q += fmt.Sprintf(" order by %s %s", opts.OrderBy.Field, opts.OrderBy.Order)
		}
	}
	return db.Query(q, values...)
}

func getFieldColumnName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup(packageTagName)
	if !ok || tag == "" || tag == "-" {
		return ""
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
	return ""
}

// QueryStruct looks up for rows in given table and scans it to provided struct or slice of structs
func QueryStruct(db *sql.DB, table string, opts *Options, out interface{}) error {
	ov := reflect.ValueOf(out)
	if ov.Type().Kind() != reflect.Ptr {
		return errors.New("ormlite: receiver is not pointer")
	}

	oe := ov.Elem()
	if oe.Type().Kind() != reflect.Struct {
		return fmt.Errorf("ormlite: expected pointer to struct, got %T", oe.Type())
	}

	var (
		pkField   reflect.Value
		columns   []string
		fieldPtrs []interface{}
		relations = make(map[*relationInfo]reflect.Value)
	)

	for i := 0; i < oe.NumField(); i++ {
		if ri := extractRelationInfo(oe.Type().Field(i)); ri != nil {
			if ri.Type == oneToOne {
				continue
			}
			relations[ri] = oe.Field(i)
			break
		}
		if c := getFieldColumnName(oe.Type().Field(i)); c != "" {
			columns = append(columns, c)
		} else {
			columns = append(columns, strings.ToLower(oe.Type().Field(i).Name))
		}
		fieldPtrs = append(fieldPtrs, oe.Field(i).Addr().Interface())

		tag, ok := oe.Type().Field(i).Tag.Lookup(packageTagName)
		if !ok {
			continue
		}
		if lookForSetting(tag, "primary") == "primary" {
			pkField = oe.Field(i)
		}
	}

	rows, err := queryWithOptions(db, table, columns, opts)
	if err != nil {
		return fmt.Errorf("ormlite: failed to perform query: %v", err)
	}

	for rows.Next() {
		if err := rows.Scan(fieldPtrs...); err != nil {
			return fmt.Errorf("ormlite: failed to scan: %v", err)
		}
	}
	if opts == nil || !opts.LoadRelations {
		return nil
	}
	// load relations
	for ri, rv := range relations {
		var (
			rPKField string
			rPKs     []interface{}
		)
		if rv.Kind() != reflect.Slice {
			return fmt.Errorf("ormlite: can't load relations: wrong field type: %v", rv.Type())
		}
		rvt := rv.Type().Elem()
		if rvt.Kind() != reflect.Ptr {
			return fmt.Errorf("ormlite: can't load relations: wrong field type: %v", rvt)
		}
		rve := rvt.Elem()
		if rve.Kind() != reflect.Struct {
			return fmt.Errorf("ormlite: can't load relations: wrong field type: %v", rve)
		}
		for i := 0; i < rve.NumField(); i++ {
			t, ok := rve.Field(i).Tag.Lookup(packageTagName)
			if !ok {
				continue
			}
			if lookForSetting(t, "primary") == "primary" {
				rPKField = lookForSetting(t, "col")
				break
			}
		}
		
		var where string 
		if ri.FieldName != "" {
			where = fmt.Sprintf("where %s = ?", ri.FieldName)
		}

		rows, err := db.Query(fmt.Sprintf("select %s from %s %s", rPKField, ri.Table, where), pkField.Interface())
		if err != nil {
			return fmt.Errorf("ormlite: failed to query for relations: %v", err)
		}
		for rows.Next() {
			var rPK int
			if err := rows.Scan(&rPK); err != nil {
				return fmt.Errorf("ormlite: failed to scan relation pk: %v", err)
			}
			rPKs = append(rPKs, rPK)
		}
		if err := QuerySlice(
			db, "", &Options{Where: map[string]interface{}{rPKField: rPKs}}, rv.Addr().Interface()); err != nil {
			return fmt.Errorf("ormlite: failed to query slice: %v", err)
		}
	}
	return nil
}

// QuerySlice scans rows into the slice of structs
func QuerySlice(db *sql.DB, table string, opts *Options, out interface{}) error {
	ov := reflect.ValueOf(out)
	if ov.Kind() != reflect.Ptr {
		return errors.New("ormlite: receiver is not pointer")
	}
	osv := ov.Elem()
	if osv.Kind() != reflect.Slice {
		return fmt.Errorf("ormlite: expected pointer to slice, got %v", osv.Kind())
	}
	ose := osv.Type().Elem()
	if ose.Kind() != reflect.Ptr {
		return fmt.Errorf("ormlite: expected slice of pointers, go %v", ose.Kind())
	}

	if table == "" {
		if m, ok := ose.MethodByName("Table"); ok {
			// TODO: find another way to check implementation of Model interface
			table = m.Func.Call([]reflect.Value{reflect.New(ose).Elem()})[0].Interface().(string)
		} else {
			return errors.New("ormlite: empty table with non model")
		}
	}

	oss := ose.Elem()
	if oss.Kind() != reflect.Struct {
		return fmt.Errorf("ormlite: expected pointer to struct, got %v", oss)
	}

	var (
		columns []string
		fi      = make(map[int]struct{})
	)
	for i := 0; i < oss.NumField(); i++ {
		if ri := extractRelationInfo(oss.Field(i)); ri != nil && ri.Type != oneToOne {
			continue
		}
		if c := getFieldColumnName(oss.Field(i)); c != "" {
			columns = append(columns, c)
		} else {
			columns = append(columns, strings.ToLower(oss.Field(i).Name))
		}
		fi[i] = struct{}{}
	}

	rows, err := queryWithOptions(db, table, columns, opts)
	if err != nil {
		return fmt.Errorf("ormlite: failed to query slice of structs: %v", err)
	}

	for rows.Next() {
		se := reflect.New(oss)

		var fptrs []interface{}
		for i := 0; i < se.Elem().NumField(); i++ {
			if _, ok := fi[i]; ok {
				fptrs = append(fptrs, se.Elem().Field(i).Addr().Interface())
			}
		}
		if err := rows.Scan(fptrs...); err != nil {
			return fmt.Errorf("ormlite: failed to scan to slice entry: %v", err)
		}
		osv.Set(reflect.Append(osv, se))
	}
	return nil
}

// Delete removes model object from database, if object was changed before saving it won't be deleted.
func Delete(db *sql.DB, m Model) error {
	s := reflect.ValueOf(m).Elem()

	var (
		columns []string
		values  []interface{}
	)

	for i := 0; i < s.NumField(); i++ {
		if c := getFieldColumnName(s.Type().Field(i)); c != "" {
			columns = append(columns, fmt.Sprintf("%s = ?", c))
		} else {
			columns = append(columns, fmt.Sprintf("%s = ?", strings.ToLower(s.Type().Field(i).Name)))
		}
		values = append(values, s.Field(i).Interface())
	}

	query := fmt.Sprintf("delete from %s where %s", m.Table(), strings.Join(columns, " and "))
	res, err := db.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("ormlite: failed to exec query: %v", err)
	}

	ra, err := res.RowsAffected()
	if err != nil || ra == 0 {
		return errors.New("ormlite: query didn't affect any rows")
	}

	return nil
}

// Upsert inserts object into table or updates it's values if it's not exist or udpates it
func Upsert(db *sql.DB, m Model) error {
	ev := reflect.ValueOf(m)
	if ev.Kind() != reflect.Ptr {
		return fmt.Errorf("ormlite: model expected to be ptr, %v given", ev.Kind())
	}

	et := ev.Elem().Type()
	if et.Kind() != reflect.Struct {
		return fmt.Errorf("ormlite: model expected to be a pointer to a struct, not to %v", et.Kind())
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
		fTag, ok := et.Field(i).Tag.Lookup(packageTagName)
		if !ok {
			fields = append(fields, strings.ToLower(et.Field(i).Name))
			values = append(values, ev.Elem().Field(i).Interface())
			continue
		}

		if strings.Contains(fTag, "primary") {
			if reflect.Zero(et.Field(i).Type).Interface() != ev.Elem().Field(i).Interface() {
				pk = ev.Elem().Field(i).Interface()
			}
			if c := getFieldColumnName(et.Field(i)); c != "" {
				pkFieldName = c
			} else {
				pkFieldName = strings.ToLower(et.Field(i).Name)
			}
			pkField = ev.Elem().Field(i)
			continue
		}

		if rInfo := extractRelationInfo(et.Field(i)); rInfo != nil {
			switch rInfo.Type {
			case noRelation, oneToMany:
				continue
			case oneToOne:
				refValue := reflect.ValueOf(ev.Elem().Field(i).Interface())
				if refValue.Kind() != reflect.Ptr {
					return fmt.Errorf("ormlite: one-to-one relations supports only pointer to struct, not %T", ev.Elem().Field(i).Interface())
				}
				var refPkFieldValue interface{}
				for i := 0; i < refValue.Elem().NumField(); i++ {
					if lookForSetting(refValue.Elem().Type().Field(i).Tag.Get(packageTagName), "primary") == "primary" {
						refPkFieldValue = refValue.Elem().Field(i).Interface()
					}
				}
				if refPkFieldValue == nil {
					return errors.New("ormlite: one-to-one related struct don't have primary key")
				}
				values = append(values, refPkFieldValue)
				fields = append(fields, getFieldColumnName(et.Field(i)))
			case manyToMany:
				relations[rInfo] = ev.Elem().Field(i).Interface()
			}
		} else {
			values = append(values, ev.Elem().Field(i).Interface())
		}
	}

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

	res, err := db.Exec(query, values...)
	if err != nil {
		return fmt.Errorf("ormlite: failed to exec: %v", err)
	}
	ra, err := res.RowsAffected()
	if err != nil || ra == 0 {
		return errors.New("ormlite: no rows were affected")
	}
	// if it was insert query - set new id to entry
	if pk == nil {
		iid, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("ormlite: failed to get last inserted id: %v", err)
		}
		if pkField.Kind() != reflect.Int {
			return errors.New("ormlite: insert functionality can be used only for models with int primary keys")
		}
		pkField.SetInt(iid)
	}
	// if there were mtm relations process them
	for rel, iface := range relations {
		if rel.Table == "" || rel.FieldName == "" {
			return errors.New("ormlite: failed to process relations: not enougth settings")
		}
		rv := reflect.ValueOf(iface)
		if rv.Kind() != reflect.Slice {
			return errors.New("ormlite: failed to process relations: wrong field type")
		}
		rvt := rv.Type().Elem()
		if rvt.Kind() != reflect.Ptr {
			return errors.New("ormlite: failed to process relations: wrong field type")
		}
		rvs := rvt.Elem()
		if rvs.Kind() != reflect.Struct {
			return errors.New("ormlite: failed to process relations: wrong field type")
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
		exgRels := map[interface{}]bool{}
		rows, err := db.Query(fmt.Sprintf("select %s from %s where %s = ?", refFieldName, rel.Table, rel.FieldName), pk)
		if err != nil {
			return fmt.Errorf("ormlite: failed to load mtm relations: %v", err)
		}
		for rows.Next() {
			var refPK int // TODO: we need some casting to support not only int PK's
			if err := rows.Scan(&refPK); err != nil {
				return fmt.Errorf("ormlite: failed to scan mtm relation primary key: %v", err)
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
						res, err := db.Exec(
							fmt.Sprintf(
								"insert into %s(%s, %s) values(?, ?)", rel.Table, rel.FieldName, refFieldName), pk, is.Field(i).Interface())
						if err != nil {
							return fmt.Errorf("ormlite: failed to add missing relation: %v", err)
						}
						ra, err := res.RowsAffected()
						if err != nil || ra == 0 {
							return fmt.Errorf("ormlite: failed to get rows affected of missing relations add or it's 0 (%v)", err)
						}
					}
					exgRels[is.Field(i).Interface()] = true
				}
			}
		}
		// delete
		for refPK, exists := range exgRels {
			if !exists {
				res, err := db.Exec(
					fmt.Sprintf(
						"delete from %s where %s = ? and %s = ?", rel.Table, rel.FieldName, refFieldName), pk, refPK)
				if err != nil {
					return fmt.Errorf("ormlite: failed to delete removed relation: %v", err)
				}
				ra, err := res.RowsAffected()
				if err != nil || ra == 0 {
					return fmt.Errorf("ormlite: failed to get rows affected of removed relations delete or it's 0 (%v)", err)
				}
			}
		}
	}

	return nil
}
