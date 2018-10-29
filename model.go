package ormlite

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/iancoleman/strcase"
	"github.com/pkg/errors"
	"reflect"
)

type IModel interface {
	Table() string
}

type fieldType int

const (
	regularField fieldType = 1 << iota
	referenceField
	pkField
)

func isPkField(field modelField) bool {
	return field.Type&pkField == pkField
}

func isReferenceField(field modelField) bool {
	return field.Type&referenceField == referenceField
}

func isZeroField(field reflect.Value) bool {
	return field.Interface() == reflect.Zero(field.Type()).Interface()
}

type modelFieldReference struct {
	Type      string
	rType     reflect.Type
	table     string
	condition string
	column    string
}

type modelField struct {
	Type      fieldType
	column    string
	reference modelFieldReference
	value     reflect.Value
}

type modelInfo struct {
	value  reflect.Value
	fields []modelField
	table  string
}

// Check if given interface is a Model or slice of Models
func getModelValue(o interface{}) (reflect.Value, error) {
	spew.Dump(o)
	value := reflect.ValueOf(o)
	fmt.Printf("settable value: %v\n", value.CanSet())
	switch value.Kind() {
	case reflect.Struct:
		if _, ok := reflect.New(value.Type()).Interface().(IModel); ok {
			return value, nil
		}
		return value, errors.New("given object does not meet Model interface")
	case reflect.Ptr:
		return getModelValue(value.Elem().Interface())
	default:
		return value, errors.Errorf("expected pointer, got %T", o)
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
		mField = modelField{Type: regularField}
		field  = mValue.Type().Field(fIndex)
		tag    = field.Tag.Get(packageTagName)
	)
	mField.column = getFieldColumnName(field)
	mField.value = mValue.Field(fIndex)
	mField.reference.rType = field.Type
	mField.reference.column = lookForSetting(tag, "ref")
	// parse references
	switch {
	case lookForSetting(tag, "many_to_many") != "":
		mField.reference.Type = "many_to_many"
		mField.reference.table = lookForSetting(tag, "table")
		mField.reference.condition = lookForSetting(tag, "condition")
		mField.Type += referenceField
	case lookForSetting(tag, "has_one") != "":
		mField.reference.Type = "has_one"
		mField.Type += referenceField
	case lookForSetting(tag, "has_many") != "":
		mField.reference.Type = "has_many"
		mField.Type += referenceField
	case lookForSetting(tag, "primary") != "":
		mField.Type += pkField
	}
	return mField, nil
}

// Parse model to obtain information useful for query builder
func getModelInfo(o interface{}) (*modelInfo, error) {
	mv, err := getModelValue(o)
	if err != nil {
		return nil, err
	}

	var mi = modelInfo{table: o.(IModel).Table()}

	for i := 0; i < mv.NumField(); i++ {
		mf, err := getFieldInfo(mv, i)
		if err != nil {
			return nil, err
		}
		mi.fields = append(mi.fields, mf)
	}
	return &mi, nil
}
