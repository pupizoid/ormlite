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
	ID    int64 `ormlite:"primary,ref=base_id"`
	Field string
}

func (*baseModel) Table() string { return "base_model" }

func (s *baseModelFixture) Query() string {
	return `
		create table base_model(id integer primary key, field text)
	`
}
func (s *baseModelFixture) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	s.db = db
}

func (s *baseModelFixture) TestInsert() {
	var m = baseModel{Field: "test"}
	if assert.NoError(s.T(), upsert(context.Background(), s.db, &m)) {
		assert.EqualValues(s.T(), 1, m.ID)
	}
}

func (s *baseModelFixture) TestUpdate() {
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

// test auto create related objects

type autoCreateRelatedFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *autoCreateRelatedFixture) Query() string {
	return `
		create table base_model(id integer primary key, field text);
		create table main_model(id integer primary key, name text, related_has_one int);
		create table has_many_model(id integer primary key, related_id integer);
		create table many_to_many_model(id integer primary key, field text);
		create table mapping_table(m_id int, m2_id int);
	`
}

func (s *autoCreateRelatedFixture) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	s.db = db
}

type autoCreateRelatedHasManyModel struct {
	ID      int64                   `ormlite:"primary,ref=has_many_id"`
	Related *autoCreateRelatedModel `ormlite:"has_one,col=related_id"`
}

func (*autoCreateRelatedHasManyModel) Table() string { return "has_many_model" }

type autoCreateRelatedManyToManyModel struct {
	ID    int64 `ormlite:"primary,ref=m2_id"`
	Field string
}

func (*autoCreateRelatedManyToManyModel) Table() string { return "many_to_many_model" }

type autoCreateRelatedModel struct {
	ID                int64 `ormlite:"primary,ref=m_id"`
	Name              string
	RelatedHasOne     *baseModel                          `ormlite:"has_one,col=related_has_one"`
	RelatedHasMany    []*autoCreateRelatedHasManyModel    `ormlite:"has_many"`
	RelatedManyToMany []*autoCreateRelatedManyToManyModel `ormlite:"many_to_many,table=mapping_table"`
}

func (*autoCreateRelatedModel) Table() string { return "main_model" }

func (s *autoCreateRelatedFixture) Test() {
	m := autoCreateRelatedModel{
		Name:              "test",
		RelatedHasOne:     &baseModel{Field: "base model field"},
		RelatedHasMany:    []*autoCreateRelatedHasManyModel{{Related: &autoCreateRelatedModel{ID: 1}}, {Related: &autoCreateRelatedModel{ID: 1}}},
		RelatedManyToMany: []*autoCreateRelatedManyToManyModel{{Field: "test 1"}, {Field: "test 2"}},
	}
	err := upsert(context.Background(), s.db, &m)
	if assert.NoError(s.T(), err) {
		// assert model was created
		assert.NotZero(s.T(), m.ID)
		// assert has_one model was created
		assert.NotZero(s.T(), m.RelatedHasOne.ID)
		assert.EqualValues(s.T(), 1, m.RelatedHasOne.ID)
		// assert has_many models were created
		for i, rhm := range m.RelatedHasMany {
			assert.EqualValues(s.T(), i+1, rhm.ID)
		}
		// assert many_to_many models were created
		//for i, rmtm := range m.RelatedManyToMany {
		//	assert.EqualValues(s.T(), i+1, rmtm.ID)
		//}

	}
}

func TestAutoCreate(t *testing.T) {
	suite.Run(t, new(autoCreateRelatedFixture))
}
