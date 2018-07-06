package ormlite

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type model struct{}

func (*model) Table() string { return "" }

type simpleModel struct {
	ID             int `ormlite:"col=rowid,primary"`
	NotTaggedField string
	TaggedField    string `ormlite:"col=tagged_field"`
	OmittedField   string `ormlite:"-"`
}

func (sm *simpleModel) Table() string { return "simple_model" }

type simpleModelFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *simpleModelFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	assert.NoError(s.T(), err)

	_, err = c.Exec(`
		create table simple_model (
			nottaggedfield text,
			tagged_field text
		);

		insert into simple_model(nottaggedfield, tagged_field) values 
			('test', 'test tagged'),
			('asdad', 'assddffgh'),
			('1111', '22222');
	`)
	assert.NoError(s.T(), err)
	s.db = c
}

func (s *simpleModelFixture) TearDownSuite() {
	s.db.Close()
}

func (s *simpleModelFixture) TestCRUD() {
	var m1 = simpleModel{TaggedField: "some tagged field"}
	assert.NoError(s.T(), Upsert(s.db, &m1))
	assert.NotZero(s.T(), m1.ID)
	m1.NotTaggedField = "some not tagged field"
	assert.NoError(s.T(), Upsert(s.db, &m1))

	var m2 simpleModel
	assert.NoError(s.T(), QueryStruct(s.db, "simple_model", &Options{Where: map[string]interface{}{"rowid": m1.ID}}, &m2))
	assert.Equal(s.T(), m1, m2)

	assert.NoError(s.T(), Delete(s.db, &m2))
}

func (s *simpleModelFixture) TestQuerySlice() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, "simple_model", nil, &mm))
	assert.NotEmpty(s.T(), mm)
}

func (s *simpleModelFixture) TestLimit() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, "simple_model", &Options{Limit: 1}, &mm))
	assert.Equal(s.T(), 1, len(mm))
}

func (s *simpleModelFixture) TestOffset() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, "simple_model", &Options{Limit: 2, Offset: 1}, &mm))
	assert.NotEmpty(s.T(), mm)
	for _, m := range mm {
		assert.NotEqual(s.T(), 1, m.ID, "First row shouldn't be returned since offset")
	}
}

func (s *simpleModelFixture) TestOrderBy() {
	var mm []*simpleModel
	require.NoError(s.T(), QuerySlice(s.db, "simple_model", &Options{OrderBy: &OrderBy{Field: "rowid", Order: "desc"}}, &mm))
	assert.NotEmpty(s.T(), mm)
	assert.NotEqual(s.T(), 1, mm[0].ID)
}

func (s *simpleModelFixture) TestInvalidModels() {
	var m struct {
		ID int
	}
	assert.Error(s.T(), QueryStruct(s.db, "", nil, m))
	assert.Error(s.T(), QueryStruct(s.db, "", nil, &m))
	assert.Error(s.T(), QuerySlice(s.db, "", nil, m))
	assert.Error(s.T(), QuerySlice(s.db, "", nil, &m))
	var m1 []struct{}
	assert.Error(s.T(), QuerySlice(s.db, "", nil, &m1))
	var m2 []*struct{}
	assert.Error(s.T(), QuerySlice(s.db, "", nil, &m2))
}

func TestSimpleModel(t *testing.T) {
	suite.Run(t, new(simpleModelFixture))
}

type relatedModel struct {
	ID    int `ormlite:"col=rowid,primary,ref=rel_id"`
	Field string
}

func (m *relatedModel) Table() string { return "related_model" }

type modelHasOne struct {
	ID      int           `ormlite:"col=rowid,primary"`
	Related *relatedModel `ormlite:"has_one,col=rel_id"`
}

func (m *modelHasOne) Table() string { return "one_to_one_rel" }

type hasOneRelationFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *hasOneRelationFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
		create table related_model ( field text );
		create table one_to_one_rel ( rel_id int references related_model (rowid) );
		
		insert into related_model (field) values('test'), ('test 2');
		insert into one_to_one_rel (rel_id) values(1), (null);
	`)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *hasOneRelationFixture) TearDownSuite() {
	s.db.Close()
}

func (s *hasOneRelationFixture) TestQueryStruct() {
	var m modelHasOne
	require.NoError(s.T(), QueryStruct(s.db, "one_to_one_rel",
		&Options{LoadRelations: true, Where: map[string]interface{}{"rowid": 1}}, &m))
	assert.Equal(s.T(), 1, m.ID)
	require.NotNil(s.T(), m.Related)
	assert.Equal(s.T(), "test", m.Related.Field)
	assert.Equal(s.T(), 1, m.Related.ID)
}

func (s *hasOneRelationFixture) TestUpsertAndDelete() {
	var m = modelHasOne{Related: &relatedModel{ID: 2, Field: "lol"}}
	require.NoError(s.T(), Upsert(s.db, &m))
	var mm []*modelHasOne
	require.NoError(s.T(), QuerySlice(s.db, m.Table(), nil, &mm))
	assert.Equal(s.T(), 3, len(mm))
	for _, m := range mm {
		assert.NoError(s.T(), QueryStruct(s.db, m.Table(), &Options{LoadRelations: true, Where: map[string]interface{}{"rowid": m.ID}}, m))
	}
	assert.Equal(s.T(), 2, mm[2].Related.ID)
	assert.Equal(s.T(), "test 2", mm[2].Related.Field)
	//
	assert.NoError(s.T(), Delete(s.db, mm[0]))
}

func TestHasOneRelation(t *testing.T) {
	suite.Run(t, new(hasOneRelationFixture))
}

type relatingModel struct {
	ID      int           `ormlite:"col=rowid,primary"`
	Related *hasManyModel `ormlite:"has_one,col=related_id"`
}

func (*relatingModel) Table() string { return "relating_model" }

type hasManyModel struct {
	ID      int `ormlite:"col=rowid,primary"`
	Name    string
	Related []*relatingModel `ormlite:"has_many"`
}

func (*hasManyModel) Table() string { return "has_many_model" }

type hasManyModelFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *hasManyModelFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)
	_, err = c.Exec(`
		create table has_many_model (name text);
		create table relating_model (related_id int references has_many_model (rowid));

		insert into has_many_model (name) values ('test');
		insert into relating_model (related_id) values (1), (1), (1);
	`)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *hasManyModelFixture) TearDownSuite() {
	s.db.Close()
}

func (s *hasManyModelFixture) TestQuery() {
	var m hasManyModel
	require.NoError(s.T(), QueryStruct(s.db, "", &Options{LoadRelations: true}, &m))
	assert.NotNil(s.T(), m.Related)
	assert.Equal(s.T(), 3, len(m.Related))
}

func TestHasManyRelation(t *testing.T) {
	suite.Run(t, new(hasManyModelFixture))
}

type modelManyToMany struct {
	ID      int `ormlite:"col=rowid,primary"`
	Name    string
	Related []*relatedModel `ormlite:"many_to_many,table=mtm,field=m_id"`
}

func (*modelManyToMany) Table() string { return "mtm_model" }

type manyToManyRelationFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *manyToManyRelationFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
		create table related_model ( field text );
		create table mtm_model ( name text );

		create table mtm (m_id int references mtm_model (rowid), rel_id int references related_model (rowid));
		insert into related_model (field) values('test 1'), ('test 2'), ('test 3');
		insert into mtm_model(name) values ('name');
		insert into mtm(m_id, rel_id) values(1, 1), (1, 2);
	`)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *manyToManyRelationFixture) TearDownSuite() {
	s.db.Close()
}

func (s *manyToManyRelationFixture) TestQuery() {
	var mm []*modelManyToMany
	require.NoError(s.T(), QuerySlice(s.db, new(modelManyToMany).Table(), nil, &mm))
	for _, m := range mm {
		assert.NoError(s.T(), QueryStruct(s.db, "mtm_model", &Options{LoadRelations: true, Where: map[string]interface{}{"rowid": m.ID}}, m))
	}
	assert.Equal(s.T(), 1, len(mm))
	assert.Equal(s.T(), 2, len(mm[0].Related))
}

func (s *manyToManyRelationFixture) TestUpsert() {
	var m = modelManyToMany{
		ID:      1,
		Name:    "name",
		Related: []*relatedModel{&relatedModel{ID: 2}, &relatedModel{ID: 3}},
	}
	require.NoError(s.T(), Upsert(s.db, &m))
	var m1 modelManyToMany
	assert.NoError(s.T(), QueryStruct(s.db, "", &Options{LoadRelations: true, Where: map[string]interface{}{"rowid": m.ID}}, &m1))
	assert.NotEmpty(s.T(), m1.Related)
	for _, r := range m1.Related {
		assert.NotEqual(s.T(), 1, r.ID)
	}
	m2 := modelManyToMany{Name: "test", Related: []*relatedModel{&relatedModel{ID: 1}}}
	require.NoError(s.T(), Upsert(s.db, &m2))
	var c int
	rows, err := s.db.Query("select count(*) from mtm")
	require.NoError(s.T(), err)
	for rows.Next() {
		require.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 3, c)
	assert.NoError(s.T(), Delete(s.db, &m2))
}
func TestManyToManyRelation(t *testing.T) {
	suite.Run(t, new(manyToManyRelationFixture))
}

type modelMultiTable struct {
	One []*relatedModel `ormlite:"many_to_many,table=one"`
	Two []*relatedModel `ormlite:"many_to_many,table=two"`
}

func (*modelMultiTable) Table() string { return "" }

type modelMultiTableFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *modelMultiTableFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
		create table related_model(field text);
		create table one(rel_id int references related_model(rowid));
		create table two(rel_id int references related_model(rowid));
		
		insert into related_model(field) values('1'), ('2'), ('3'), ('4'), ('5');
		insert into one(rel_id) values (1), (2), (3);
		insert into two(rel_id) values (4), (5);
	`)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *modelMultiTableFixture) TearDownSuite() {
	s.db.Close()
}

func (s *modelMultiTableFixture) TestQuery() {
	var m modelMultiTable
	require.NoError(s.T(), QueryStruct(s.db, "", &Options{LoadRelations: true}, &m))
	assert.Equal(s.T(), 3, len(m.One))
	assert.Equal(s.T(), 2, len(m.Two))
}

func (s *modelMultiTableFixture) TestUpsert() {
	m := modelMultiTable{
		One: []*relatedModel{&relatedModel{ID: 1}},
		Two: []*relatedModel{&relatedModel{ID: 1}, &relatedModel{ID: 2}, &relatedModel{ID: 3}},
	}
	require.NoError(s.T(), Upsert(s.db, &m))
	var c int
	rows, err := s.db.Query("select count(*) from two")
	require.NoError(s.T(), err)
	for rows.Next() {
		assert.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 3, c)
}

func (s *modelMultiTableFixture) TestDelete() {
	assert.Error(s.T(), Delete(s.db, new(modelMultiTable)))
}

func TestMultiTableModel(t *testing.T) {
	suite.Run(t, new(modelMultiTableFixture))
}
