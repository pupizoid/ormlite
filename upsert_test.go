package ormlite

import (
	"database/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type baseModelFixture struct {
	suite.Suite
	db *sql.DB
}

type baseModel struct {
	ID    int64 `ormlite:"primary"`
	Field string
}

func (*baseModel) Table() string { return "base_model" }

func (s *baseModelFixture) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	_, err = db.Exec(`
		create table base_model(id integer primary key, field text)
	`)
	require.NoError(s.T(), err)
	s.db = db
}

func (s *baseModelFixture) TestInsert() {
	var m = baseModel{Field: "test"}
	err := upsert(s.db, &m)
	if assert.NoError(s.T(), err) {
		assert.EqualValues(s.T(), 1, m.ID)
	}
}

func TestBaseModel(t *testing.T) {
	suite.Run(t, new(baseModelFixture))
}
