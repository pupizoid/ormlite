package ormlite

import (
	"context"
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
	if assert.NoError(s.T(), upsert(context.Background(), s.db, &m)) {
		assert.EqualValues(s.T(), 1, m.ID)
	}
}

func (s *baseModelFixture) TestUpsert() {
	var m = baseModel{ID: 1, Field: "test 2"}
	if assert.NoError(s.T(), upsert(context.Background(), s.db, &m)) {
		// check db really changed
		rows, err := s.db.Query("select field from base_model where id = ?", m.ID)
		if assert.NoError(s.T(), err) {
			for rows.Next() {
				var field string
				assert.NoError(s.T(), rows.Scan(&field))
				assert.EqualValues(s.T(), m.Field, field)
			}
		}
	}
}

func TestBaseModel(t *testing.T) {
	suite.Run(t, new(baseModelFixture))
}
