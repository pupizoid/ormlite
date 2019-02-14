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
		Name:          "test",
		RelatedHasOne: &baseModel{Field: "base model field"},
		RelatedHasMany: []*autoCreateRelatedHasManyModel{
			{Related: &autoCreateRelatedModel{ID: 1}},
			{Related: &autoCreateRelatedModel{ID: 1}}},
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
	}
}

func TestAutoCreate(t *testing.T) {
	suite.Run(t, new(autoCreateRelatedFixture))
}

type uniqueFieldFixture struct {
	suite.Suite
	db *sql.DB
}

type modelWithUniqueField struct {
	ID    int64  `ormlite:"primary"`
	Field string `ormlite:"unique"`
}

func (*modelWithUniqueField) Table() string { return "test_unique" }

func (s *uniqueFieldFixture) Query() string {
	return `
		create table test_unique(id integer primary key, field text unique);
		insert into test_unique(field) values ('test 1'), ('test 2'), ('test 3');
	`
}

func (s *uniqueFieldFixture) SetupSuite() {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	_, err = db.Query("select count(*) from test_unique")
	require.NoError(s.T(), err)
	s.db = db
}

func (s *uniqueFieldFixture) TestUpsert() {
	m := modelWithUniqueField{Field: "test 3"}
	if assert.NoError(s.T(), Upsert(s.db, &m)) {
		assert.EqualValues(
			s.T(), 3, m.ID, "ID should be equal to the row that caused unique violation")
	}
	m2 := modelWithUniqueField{Field: "test 2"}
	if assert.NoError(s.T(), Upsert(s.db, &m2)) {
		assert.EqualValues(
			s.T(), 2, m2.ID, "ID should be equal to the row that caused unique violation")
	}
}

func TestUniqueField(t *testing.T) {
	suite.Run(t, new(uniqueFieldFixture))
}

type skipUpdatingExistingRelatedModels struct {
	suite.Suite
	db *sql.DB
}

type skipHasOneModel struct {
	ID      int64             `ormlite:"primary,col=rowid"`
	Related *skipHasManyModel `ormlite:"has_one,col=related_id"`
}

func (*skipHasOneModel) Table() string { return "has_one_model" }

type skipHasManyModel struct {
	ID      int64                `ormlite:"primary,col=rowid"`
	Related []*skipRelatingModel `ormlite:"has_many"`
}

func (*skipHasManyModel) Table() string { return "has_many_model" }

type skipRelatingModel struct {
	ID      int64             `ormlite:"primary,col=rowid"`
	Related *skipHasManyModel `ormlite:"has_one,col=related_id"`
}

func (*skipRelatingModel) Table() string { return "relating_model" }

func (s *skipUpdatingExistingRelatedModels) Query() string {
	return `
		create table relating_model (related_id int);
		create table has_many_model (name text);
		create table has_one_model (related_id int);

		insert into has_many_model (name) values ('test');
		insert into relating_model (related_id) values (1), (1), (1);
		insert into has_one_model (related_id) values (1);
	`
}

func (s *skipUpdatingExistingRelatedModels) SetupSuite() {
	db, err := sql.Open("sqlite3", "file::memory:?cache=shared")
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	s.db = db
}

func (s *skipUpdatingExistingRelatedModels) Test() {
	var m = skipHasOneModel{1, &skipHasManyModel{
		ID: 1, Related: []*skipRelatingModel{
			{1, nil}, {2, nil}, {3, nil},
		}},
	}

	if assert.NoError(s.T(), Upsert(s.db, &m)) {
		// query relating model
		var rms []*skipRelatingModel
		if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelationDepth: 2}, &rms)) {
			assert.Equal(s.T(), 3, len(rms))
			for _, rm := range rms {
				assert.NotNil(s.T(), rm.Related)
			}
		}
	}
}

func TestSkipUpdatingExistingModels(t *testing.T) {
	suite.Run(t, new(skipUpdatingExistingRelatedModels))
}
