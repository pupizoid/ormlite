package ormlite

import (
	"database/sql"
	"database/sql/driver"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type model struct{}

func (m *model) Table() string { return "" }

func TestGetModelValue(t *testing.T) {
	_, err := getModelValue(&model{})
	assert.NoError(t, err)
	_, err = getModelValue(&[]*model{})
	assert.NoError(t, err)
	_, err = getModelValue(&[]struct{}{})
	assert.Error(t, err)
}

type countField struct {
	value int64
}

func (cf *countField) Scan(src interface{}) error {
	switch src.(type) {
	case int64:
		cf.value = src.(int64)
	default:
		return errors.New("unsupported count type")
	}
	return nil
}

func (cf *countField) Value() (driver.Value, error) {
	if cf == nil {
		return nil, nil
	}
	return driver.Int32.ConvertValue(cf.value)
}

func (cf *countField) Column() string {
	return "(select count(*) from test) as count"
}

type modelWithCount struct {
	ID    int64 `ormlite:"primary"`
	Name  string
	Count *countField
}

func (m *modelWithCount) Table() string { return "test" }

type expressionFieldFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *expressionFieldFixture) Query() string {
	return `
		create table test(id integer primary key, name text);
		insert into test(name) values ('1'),('2'),('3'),('4'),('5');
`
}

func (s *expressionFieldFixture) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	s.db = db
}

func (s *expressionFieldFixture) TestQuery() {
	var mm []*modelWithCount
	if assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm)) {
		assert.Len(s.T(), mm, 5)
		for _, m := range mm {
			assert.EqualValues(s.T(), 5, m.Count.value)
		}
	}
}

func (s *expressionFieldFixture) TestUpdate() {
	var m = modelWithCount{ID: 1, Name: "10", Count: nil}
	assert.NoError(s.T(), Upsert(s.db, &m))

	var mn modelWithCount
	if assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"id": 1}), &mn)) {
		assert.EqualValues(s.T(), "10", mn.Name)
		assert.EqualValues(s.T(), 5, mn.Count.value)
	}
}

func TestExpressionFields(t *testing.T) {
	suite.Run(t, new(expressionFieldFixture))
}
