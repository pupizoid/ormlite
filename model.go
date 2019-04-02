package ormlite

import (
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
	"reflect"
	"strings"
)

type IModel interface {
	Table() string
}

type fieldType int

const (
	regularField fieldType = 1 << iota
	referenceField
	omittedField
	pkField
	uniqueField
)

func isUniqueField(field modelField) bool {
	return field.Type&uniqueField == uniqueField
}

func isPkField(field modelField) bool {
	return field.Type&pkField == pkField
}

func isReferenceField(field modelField) bool {
	return field.Type&referenceField == referenceField
}

func isZeroField(field reflect.Value) bool {
	return field.Interface() == reflect.Zero(field.Type()).Interface()
}

func isOmittedField(field modelField) bool {
	return field.Type&omittedField == omittedField
}

func isHasOne(field modelField) bool {
	return field.reference.Type == "has_one"
}

func isHasMany(field modelField) bool {
	return field.reference.Type == "has_many"
}

func isManyToMany(field modelField) bool {
	return field.reference.Type == "many_to_many"
}

type fieldReference struct {
	Type      string
	rType     reflect.Type
	table     string
	condition string
	column    string
}

type modelField struct {
	Type      fieldType
	column    string
	unique    bool
	reference fieldReference
	value     reflect.Value
}

type modelInfo struct {
	value  reflect.Value
	fields []modelField
	table  string
}

// Check if given interface is a Model or slice of Models
func getModelValue(o interface{}) (reflect.Value, error) {
	value, ok := o.(reflect.Value)
	if !ok {
		value = reflect.ValueOf(o)
	}
	switch value.Kind() {
	case reflect.Struct:
		if _, ok := reflect.New(value.Type()).Interface().(IModel); ok {
			return value, nil
		}
		return value, errors.New("given object does not meet Model interface")
	case reflect.Ptr:
		return getModelValue(value.Elem())
	case reflect.Slice:
		if value.Len() == 0 {
			elemType := value.Type().Elem()
			if elemType.Kind() == reflect.Ptr {
				return getModelValue(reflect.New(elemType.Elem()).Elem())
			}
			return value, errors.Errorf("slice should contain pointers to model")
		}
		return getModelValue(value.Index(0))
	default:
		return value, errors.Errorf("expected pointer to model, got %T (kind: %v)", o, value.Kind())
	}
}

// Parses field column name, if `col` attribute was not found returns snake case
// representation of field name
func getFieldColumnName(field reflect.StructField) string {
	tag, ok := field.Tag.Lookup(packageTagName)
	if ok && tag != "" {
		if col := lookForSetting(tag, "col"); col != "" && col != "col" {
			return col
		}
	}
	return strcase.ToSnake(field.Name)
}

func getFieldInfo(mValue reflect.Value, fIndex int) (modelField, error) {
	var (
		mField = modelField{}
		field  = mValue.Type().Field(fIndex)
		tag    = field.Tag.Get(packageTagName)
	)
	mField.column = getFieldColumnName(field)
	mField.value = mValue.Field(fIndex)
	mField.reference.rType = field.Type
	// parse references
	switch {
	case lookForSetting(tag, "many_to_many") != "":
		mField.reference.Type = "many_to_many"
		mField.reference.table = lookForSetting(tag, "table")
		mField.reference.condition = lookForSettingWithSep(tag, "condition", ":")
		mField.Type += referenceField
	case lookForSetting(tag, "has_many") != "":
		mField.reference.Type = "has_many"
		mField.Type += referenceField
	case lookForSetting(tag, "has_one") != "":
		mField.reference.Type = "has_one"
		mField.Type += referenceField
	case tag == "-":
		mField.Type += omittedField
	default:
		mField.Type += regularField
	}
	if lookForSetting(tag, "primary") != "" {
		mField.reference.column = lookForSetting(tag, "ref")
		mField.Type += pkField
	}
	if lookForSetting(tag, "unique") != "" {
		mField.Type += uniqueField
	}
	return mField, nil
}

// Parse model to obtain information useful for query builder
func getModelInfo(o interface{}) (*modelInfo, error) {
	mv, err := getModelValue(o)
	if err != nil {
		return nil, err
	}

	var mi = modelInfo{
		table: reflect.New(mv.Type()).Interface().(IModel).Table(),
		value: mv,
	}

	for i := 0; i < mv.NumField(); i++ {
		if !mv.Field(i).CanInterface() {
			continue // skip unexported fields
		}
		mf, err := getFieldInfo(mv, i)
		if err != nil {
			return nil, err
		}
		mi.fields = append(mi.fields, mf)
	}
	return &mi, nil
}

func setModelPk(info *modelInfo, id int64) error {
	// check if there were last inserted id and apply it to primary key
	for _, field := range info.fields {
		if isPkField(field) && !isReferenceField(field) {
			if isZeroField(field.value) {
				field.value.SetInt(id)
			}
		}
	}
	return nil
}

// Returns pointer to a int64 value as a primary key of referenced model,
// if model does not have primary field or it's not int64 type or is a zero
// value nil will be returned.
func getRefModelPk(field modelField) *int64 {
	if field.value.IsNil() {
		return nil
	}
	mi, err := getModelInfo(field.value.Interface())
	if err != nil {
		return nil
	}
	for _, field := range mi.fields {
		if isPkField(field) {
			if !isZeroField(field.value) {
				if field.value.Kind() == reflect.Int64 {
					return field.value.Addr().Interface().(*int64)
				}
			}
		}
	}
	return nil
}

func getModelPkKeys(o interface{}) ([]interface{}, error) {
	mi, err := getModelInfo(o)
	if err != nil {
		return nil, err
	}
	var keys []interface{}
	for _, field := range mi.fields {
		if isPkField(field) {
			if isHasOne(field) {
				sub, err := getModelPkKeys(field.value)
				if err != nil {
					return nil, err
				}
				keys = append(keys, sub...)
			} else {
				keys = append(keys, field.value.Interface())
			}
		}
	}
	return keys, nil
}

func extractConditionValue(s string) (string, interface{}) {
	var (
		cond  = strings.Split(s, "=")
		field string
		value interface{}
	)
	field = cond[0]
	if len(cond) > 1 {
		if cond[1] != "" {
			if strings.Contains(cond[1], "\"") {
				value = cast.ToString(cond[1])
			} else {
				value = cast.ToInt64(cond[1])
			}
		}
	}
	return field, value
}

func getModelColumns(fields []modelField) ([]string, []string, []interface{}) {
	var (
		columns, indexes []string
		args             []interface{}
	)
	for _, field := range fields {
		if isOmittedField(field) ||
			isReferenceField(field) && !isHasOne(field) {
			continue
		}
		if isPkField(field) {
			if isZeroField(field.value) {
				continue
			}
			indexes = append(indexes, field.column)
		}
		if isUniqueField(field) {
			indexes = append(indexes, field.column)
		}
		columns = append(columns, field.column)
		if isHasOne(field) {
			args = append(args, getRefModelPk(field))
		} else {
			args = append(args, field.value.Interface())
		}
	}
	return columns, indexes, args
}

func pkIsNull(info *modelInfo) bool {
	for _, field := range info.fields {
		if isPkField(field) {
			if reflect.Zero(field.value.Type()).Interface() == field.value.Interface() {
				return true
			}
		}
	}
	return false
}
