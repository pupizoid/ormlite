package ormlite

import (
	"context"
	"database/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"io/ioutil"
	"os"
	"testing"
)

type baseModelFixture struct {
	suite.Suite
	db *sql.DB
	f  *os.File
}

type baseModel struct {
	ID    int64 `ormlite:"primary,ref=base_id"`
	Field string
}

func (*baseModel) Table() string { return "base_model" }

func (s *baseModelFixture) Query() string {
	return `
		create table base_model(id integer primary key, field text unique)
	`
}

func (s *baseModelFixture) SetupSuite() {
	s.f, _ = ioutil.TempFile("", "")

	db, err := sql.Open("sqlite3", s.f.Name())
	require.NoError(s.T(), err)
	_, err = db.Exec(s.Query())
	require.NoError(s.T(), err)
	s.db = db
}

func (s *baseModelFixture) TearDownSuite() {

}

func (s *baseModelFixture) TestAInsert() {
	var m = baseModel{Field: "test"}
	if assert.NoError(s.T(), Insert(s.db, &m)) {
		assert.EqualValues(s.T(), 1, m.ID)
	}

	var m1 = baseModel{Field: "test"}
	err := insert(context.Background(), s.db, &m1, false)
	if assert.Error(s.T(), err) {
		assert.True(s.T(), IsUniqueViolation(err))
	}
}

func (s *baseModelFixture) TestUpsert() {
	var m = baseModel{ID: 1, Field: "test 2"}
	if assert.NoError(s.T(), insert(context.Background(), s.db, &m, true)) {
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

func (s *baseModelFixture) TestUpdate() {
	var m = baseModel{ID: 1, Field: "test updateConflict"}
	err := Update(s.db, &m)
	if assert.NoError(s.T(), err) {
		rows, err := s.db.Query("select field from base_model where id = ?", m.ID)
		if assert.NoError(s.T(), err) {
			for rows.Next() {
				var field string
				assert.NoError(s.T(), rows.Scan(&field))
				assert.EqualValues(s.T(), m.Field, field)
			}
		}
	}

	m = baseModel{ID: 10, Field: "test updateConflict"}
	err = Update(s.db, &m)
	if assert.Error(s.T(), err) {
		assert.True(s.T(), IsNotFound(err))
		assert.False(s.T(), IsUniqueViolation(err))
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
		create table main_model(id integer primary key, name text, related_to int);
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
	ID      int64                   `ormlite:"primary,ref=hm_id"`
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
	RelatedHasOne     *baseModel                          `ormlite:"has_one,col=related_to"`
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
	err := insert(context.Background(), s.db, &m, true)
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
	var mm autoCreateRelatedModel
	if assert.NoError(s.T(), QueryStruct(s.db, &Options{
		RelationDepth: 4,
	}, &mm)) {
		assert.EqualValues(s.T(), 1, mm.ID)
		assert.EqualValues(s.T(), "test", mm.Name)
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
	ID              int64                `ormlite:"primary,col=rowid"`
	HasManyModel    *skipHasManyModel    `ormlite:"has_one,col=related_hm_id"`
	ManyToManyModel *skipManyToManyModel `ormlite:"has_one,col=related_mm_id"`
}

func (*skipHasOneModel) Table() string { return "has_one_model" }

type skipHasManyModel struct {
	ID      int64                `ormlite:"primary,col=rowid"`
	Related []*skipRelatingModel `ormlite:"has_many"`
}

func (*skipHasManyModel) Table() string { return "has_many_model" }

type skipRelatingModel struct {
	ID      int64             `ormlite:"primary,ref=m2_id"`
	Related *skipHasManyModel `ormlite:"has_one,col=related_id"`
}

func (*skipRelatingModel) Table() string { return "relating_model" }

type skipManyToManyModel struct {
	ID      int64                `ormlite:"primary,ref=m_id"`
	Related []*skipRelatingModel `ormlite:"many_to_many,table=mapping_table"`
}

func (*skipManyToManyModel) Table() string { return "mtm_model" }

func (s *skipUpdatingExistingRelatedModels) Query() string {
	return `
		create table relating_model (id integer primary key, related_id int);
		create table has_many_model (name text);
		create table has_one_model (related_hm_id int, related_mm_id int);
		create table mtm_model (id integer primary key);
		create table mapping_table(m_id int, m2_id int);

		insert into has_many_model (name) values ('test');
		insert into mtm_model (id) values (1);
		insert into relating_model (related_id) values (1), (1), (1);
		insert into has_one_model (related_hm_id, related_mm_id) values (1, 1);
		insert into mapping_table (m_id, m2_id) values (1,1), (1,2), (1,3);
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
			nil,
		}}, &skipManyToManyModel{1, nil},
	}

	if assert.NoError(s.T(), Upsert(s.db, &m)) {
		var rm []*skipRelatingModel
		if assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &rm)) {
			for _, rmm := range rm {
				assert.NotNil(s.T(), rmm.Related)
			}
		}

		var mm skipManyToManyModel
		if assert.NoError(s.T(), QueryStruct(s.db, &Options{RelationDepth: 4}, &mm)) {
			assert.NotNil(s.T(), mm.Related)
			assert.EqualValues(s.T(), 3, len(mm.Related))
		}
	}
}

func TestSkipUpdatingExistingModels(t *testing.T) {
	suite.Run(t, new(skipUpdatingExistingRelatedModels))
}
