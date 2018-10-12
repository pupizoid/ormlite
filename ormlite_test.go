package ormlite

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type simpleModel struct {
	ID               int `ormlite:"col=rowid,primary"`
	NotTaggedField   string
	TaggedField      string `ormlite:"col=tagged_field"`
	OmittedField     string `ormlite:"-"`
	notExportedField string
}

func (*simpleModel) Table() string { return "simple_model" }

var _ Model = (*simpleModel)(nil)

type simpleModelWithRelation struct {
	ID             int `ormlite:"col=rowid,primary"`
	NotTaggedField string
	Related        *simpleModel `ormlite:"has_one,col=related_id"`
}

func (sm *simpleModelWithRelation) Table() string { return "simple_model_has_one" }

type simpleModelWithCycleRelation struct {
	ID             int `ormlite:"col=rowid,primary"`
	NotTaggedField string
	Related        *simpleModelWithCycleRelation `ormlite:"has_one,col=related_id"`
}

func (sm *simpleModelWithCycleRelation) Table() string { return "simple_model_has_one_cycle" }

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
				
				create table simple_model_has_one (
					nottaggedfield text,
					related_id int
				);

				create table simple_model_has_one_cycle (
					nottaggedfield text,
					related_id int
				);

                insert into simple_model(nottaggedfield, tagged_field) values
					('test', 'test tagged'),
					('asdad', 'assddffgh'),
					('1111', '22222');
						
				insert into simple_model_has_one(nottaggedfield, related_id) values 
					('test', null),
					('test 2 ', 1),
					('test 3 ', 2);

				insert into simple_model_has_one_cycle(nottaggedfield, related_id) values 
					('test', 1);
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
	assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": m1.ID}), &m2))
	assert.Equal(s.T(), m1, m2)

	assert.NoError(s.T(), Delete(s.db, &m2))

	var m3 = simpleModelWithRelation{NotTaggedField: "some not tagged field"}
	assert.NoError(s.T(), Upsert(s.db, &m3))
	// test foreign key is set to null
	var m4 simpleModelWithRelation
	assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": m3.ID}), &m4))
	assert.Nil(s.T(), m4.Related)
}

func (s *simpleModelFixture) TestDeleteMissing() {
	err := Delete(s.db, &simpleModel{ID: 4})
	if assert.Error(s.T(), err) {
		assert.Equal(s.T(), ErrNoRowsAffected, err)
	}
}

func (s *simpleModelFixture) TestQuerySlice() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, nil, &mm))
	assert.NotEmpty(s.T(), mm)
}

func (s *simpleModelFixture) TestQuerySliceWithRelations() {
	var mm []*simpleModelWithRelation
	assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm))
	assert.NotEmpty(s.T(), mm)
	assert.Nil(s.T(), mm[0].Related)
	if assert.NotNil(s.T(), mm[1].Related) {
		assert.Equal(s.T(), mm[1].Related.TaggedField, "test tagged")
	}
	if assert.NotNil(s.T(), mm[2].Related) {
		assert.Equal(s.T(), mm[2].Related.TaggedField, "assddffgh")
	}
}

func (s *simpleModelFixture) TestQuerySliceWithCycleRelation() {
	var mm []*simpleModelWithCycleRelation
	assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm))
	if assert.NotEmpty(s.T(), mm) {
		assert.NotNil(s.T(), mm[0].Related)
		assert.Nil(s.T(), mm[0].Related.Related)
	}
}

func (s *simpleModelFixture) TestLimit() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, WithLimit(DefaultOptions(), 1), &mm))
	assert.Equal(s.T(), 1, len(mm))
}

func (s *simpleModelFixture) TestOffset() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, WithOffset(WithLimit(DefaultOptions(), 2), 1), &mm))
	assert.NotEmpty(s.T(), mm)
	for _, m := range mm {
		assert.NotEqual(s.T(), 1, m.ID, "First row shouldn't be returned since offset")
	}
}

func (s *simpleModelFixture) TestOrderBy() {
	var mm []*simpleModel
	require.NoError(s.T(), QuerySlice(s.db, WithOrder(DefaultOptions(), OrderBy{Field: "rowid", Order: "desc"}), &mm))
	assert.NotEmpty(s.T(), mm)
	assert.NotEqual(s.T(), 1, mm[0].ID)
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

type modelHasOneCycle struct {
	ID      int               `ormlite:"col=rowid,primary"`
	Related *modelHasOneCycle `ormlite:"has_one,col=rel_id"`
}

func (m *modelHasOneCycle) Table() string { return "one_to_one_cycle_rel" }

type hasOneRelationFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *hasOneRelationFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
                create table related_model ( field text );
                create table one_to_one_rel ( rel_id int );
				create table one_to_one_cycle_rel (rel_id int);

                insert into related_model (field) values('test'), ('test 2');
                insert into one_to_one_rel (rel_id) values(1), (null);
				insert into one_to_one_cycle_rel (rel_id) values (1);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *hasOneRelationFixture) TearDownSuite() {
	s.db.Close()
}

func (s *hasOneRelationFixture) TestQueryStruct() {
	var m modelHasOne
	require.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": 1}), &m))
	assert.Equal(s.T(), 1, m.ID)
	require.NotNil(s.T(), m.Related)
	assert.Equal(s.T(), "test", m.Related.Field)
	assert.Equal(s.T(), 1, m.Related.ID)
}

func (s *hasOneRelationFixture) TestUpsertAndDelete() {
	var m = modelHasOne{Related: &relatedModel{ID: 2, Field: "lol"}}
	require.NoError(s.T(), Upsert(s.db, &m))
	var mm []*modelHasOne
	require.NoError(s.T(), QuerySlice(s.db, nil, &mm))
	assert.Equal(s.T(), 3, len(mm))
	for _, m := range mm {
		assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": m.ID}), m))
	}
	assert.Equal(s.T(), 2, mm[2].Related.ID)
	assert.Equal(s.T(), "test 2", mm[2].Related.Field)
	//
	assert.NoError(s.T(), Delete(s.db, mm[0]))
}

func (s *hasOneRelationFixture) TestRelationalDepth() {
	var cm modelHasOneCycle
	require.NoError(s.T(), QueryStruct(s.db, &Options{RelationDepth: 2}, &cm))
	assert.NotNil(s.T(), cm.Related.Related)
	assert.Nil(s.T(), cm.Related.Related.Related)
	//
	var cms []*modelHasOneCycle
	require.NoError(s.T(), QuerySlice(s.db, &Options{RelationDepth: 2}, &cms))
	assert.NotNil(s.T(), cms[0].Related.Related)
	assert.Nil(s.T(), cms[0].Related.Related.Related)
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
                create table relating_model (related_id int);

                insert into has_many_model (name) values ('test'), ('asds');
                insert into relating_model (related_id) values (1), (1), (1), (2), (2);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *hasManyModelFixture) TearDownSuite() {
	s.db.Close()
}

func (s *hasManyModelFixture) TestQueryStruct() {
	var m hasManyModel
	require.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"name": "test"}), &m))
	assert.NotNil(s.T(), m.Related)
	assert.Equal(s.T(), 3, len(m.Related))
}

func (s *hasManyModelFixture) TestQuerySlice() {
	var mm []*hasManyModel
	require.NoError(s.T(), QuerySlice(s.db, WithWhere(DefaultOptions(), Where{"name": "asds"}), &mm))
	assert.NotEmpty(s.T(), mm)
	for _, m := range mm {
		if assert.NotEmpty(s.T(), m.Related) {
			assert.Equal(s.T(), 2, len(m.Related))
		}
	}
}

func TestHasManyRelation(t *testing.T) {
	suite.Run(t, new(hasManyModelFixture))
}

type relatingModelWithCustomPK struct {
	ID    int `ormlite:"primary,ref=c_rel_id"`
	Field string
}

func (*relatingModelWithCustomPK) Table() string { return "relating_model_custom_pk" }

type modelManyToMany struct {
	ID      int `ormlite:"col=rowid,primary"`
	Name    string
	Related []*relatedModel `ormlite:"many_to_many,table=mtm,field=m_id"`
}

func (*modelManyToMany) Table() string { return "mtm_model" }

type modelManyToManyWithCondition struct {
	ID           int `ormlite:"col=rowid,primary"`
	Name         string
	RelatedFalse []*relatedModel `ormlite:"many_to_many,table=mtm_with_condition(value=0),field=m_id"`
	RelatedTrue  []*relatedModel `ormlite:"many_to_many,table=mtm_with_condition(value=1),field=m_id"`
}

func (*modelManyToManyWithCondition) Table() string { return "mtm_model" }

type modelManyToManyWithCustomPK struct {
	ID      int `ormlite:"col=rowid,primary"`
	Name    string
	Related []*relatingModelWithCustomPK `ormlite:"many_to_many,table=mtm_with_custom_model,field=m_id"`
}

func (*modelManyToManyWithCustomPK) Table() string { return "mtm_model" }

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
                create table mtm (m_id int, rel_id int);
                
                insert into related_model (field) values('test 1'), ('test 2'), ('test 3');
                insert into mtm_model(name) values ('name');
                insert into mtm(m_id, rel_id) values(1, 1), (1, 2);

				create table mtm_with_condition ( m_id int, rel_id int, value boolean not null );
				insert into mtm_with_condition (m_id, rel_id, value) values (1,1,true), (1,3,true), (1,1,false), (1,2,true);

				create table relating_model_custom_pk(id integer primary key, field text);
				insert into relating_model_custom_pk(field) values ('common test 1'), ('common test 2');
				create table mtm_with_custom_model (m_id int, c_rel_id int);
				insert into mtm_with_custom_model(m_id, c_rel_id) values (1,1), (1,2);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *manyToManyRelationFixture) TearDownSuite() {
	s.db.Close()
}

func (s *manyToManyRelationFixture) TestQueryStruct() {
	// test regular mtm relation
	var m modelManyToMany
	require.NoError(s.T(), QueryStruct(
		s.db, WithWhere(DefaultOptions(), Where{"name": "name"}), &m))
	assert.NotEmpty(s.T(), m.Related)
	assert.Equal(s.T(), 2, len(m.Related))
	// test mtm relation with condition
	var mc modelManyToManyWithCondition
	if assert.NoError(s.T(), QueryStruct(
		s.db, WithWhere(DefaultOptions(), Where{"name": "name"}), &mc)) {
		assert.Equal(s.T(), modelManyToManyWithCondition{
			ID: 1, Name: "name",
			RelatedFalse: []*relatedModel{{1, "test 1"}},
			RelatedTrue:  []*relatedModel{{1, "test 1"}, {2, "test 2"}, {3, "test 3"}},
		}, mc)
	}
	// test mtm to model with custom primary key
	var mcc modelManyToManyWithCustomPK
	if assert.NoError(s.T(), QueryStruct(
		s.db, WithWhere(DefaultOptions(), Where{"name": "name"}), &mcc)) {
		assert.Equal(s.T(), modelManyToManyWithCustomPK{
			ID: 1, Name: "name",
			Related: []*relatingModelWithCustomPK{{1, "common test 1"}, {2, "common test 2"}},
		}, mcc)
	}
}

func (s *manyToManyRelationFixture) TestQuerySlice() {
	// test regular mtm relation
	var mm []*modelManyToMany
	require.NoError(s.T(), QuerySliceContext(context.Background(), s.db, DefaultOptions(), &mm))
	for _, m := range mm {
		assert.NotEmpty(s.T(), m.Related)
		assert.Equal(s.T(), 2, len(m.Related))
	}
	// test mtm relation with condition
	var mmc []*modelManyToManyWithCondition
	if assert.NoError(s.T(), QuerySliceContext(context.Background(), s.db, DefaultOptions(), &mmc)) {
		for _, m := range mmc {
			assert.Equal(s.T(), &modelManyToManyWithCondition{
				ID: 1, Name: "name",
				RelatedFalse: []*relatedModel{{1, "test 1"}},
				RelatedTrue:  []*relatedModel{{1, "test 1"}, {2, "test 2"}, {3, "test 3"}},
			}, m)
		}
	}
	// test mtm to model with custom primary key
	var mmcc []*modelManyToManyWithCustomPK
	if assert.NoError(s.T(), QuerySliceContext(context.Background(), s.db, DefaultOptions(), &mmcc)) {
		for _, m := range mmcc {
			assert.Equal(s.T(), &modelManyToManyWithCustomPK{
				ID: 1, Name: "name",
				Related: []*relatingModelWithCustomPK{{1, "common test 1"}, {2, "common test 2"}},
			}, m)
		}
	}
}

func (s *manyToManyRelationFixture) TestUpsert() {
	var m = modelManyToMany{
		ID:      1,
		Name:    "name",
		Related: []*relatedModel{{ID: 2}, {ID: 3}},
	}
	require.NoError(s.T(), Upsert(s.db, &m))
	var m1 modelManyToMany
	assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": m.ID}), &m1))
	assert.NotEmpty(s.T(), m1.Related)
	for _, r := range m1.Related {
		assert.NotEqual(s.T(), 1, r.ID)
	}
	m2 := modelManyToMany{Name: "test", Related: []*relatedModel{{ID: 1}}}
	require.NoError(s.T(), Upsert(s.db, &m2))
	var c int
	rows, err := s.db.Query("select count(*) from mtm")
	require.NoError(s.T(), err)
	for rows.Next() {
		require.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 3, c)
	assert.NoError(s.T(), Delete(s.db, &m2))
	// insert new model
	var m3 = modelManyToMany{
		Name:    "new model",
		Related: []*relatedModel{{ID: 3}},
	}
	assert.NoError(s.T(), Upsert(s.db, &m3))
	assert.Equal(s.T(), 2, m3.ID)
	// check upsert with condition
	var mc = modelManyToManyWithCondition{
		ID:           1,
		Name:         "name",
		RelatedFalse: []*relatedModel{{1, "test 1"}, {3, "test 3"}},
		RelatedTrue:  []*relatedModel{{2, "test 2"}, {3, "test 3"}},
	}
	require.NoError(s.T(), Upsert(s.db, &mc))
	var mc1 modelManyToManyWithCondition
	if assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": mc.ID}), &mc1)) {
		assert.Equal(s.T(), mc, mc1)
	}
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
	require.NoError(s.T(), QueryStruct(s.db, DefaultOptions(), &m))
	assert.Equal(s.T(), 3, len(m.One))
	assert.Equal(s.T(), 2, len(m.Two))
}

func (s *modelMultiTableFixture) TestUpsert() {
	m := modelMultiTable{
		One: []*relatedModel{{ID: 1}},
		Two: []*relatedModel{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	require.NoError(s.T(), Upsert(s.db, &m))
	var c int
	rows, err := s.db.Query("select count(*) from two")
	require.NoError(s.T(), err)
	for rows.Next() {
		assert.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 3, c)
	// test nullable multi table
	m1 := modelMultiTable{
		One: []*relatedModel{{ID: 1}},
	}
	require.NoError(s.T(), Upsert(s.db, &m1))
}

func (s *modelMultiTableFixture) TestDelete() {
	assert.Error(s.T(), Delete(s.db, new(modelMultiTable)))
}

func TestMultiTableModel(t *testing.T) {
	suite.Run(t, new(modelMultiTableFixture))
}

type modelWithoutPK struct {
	ID int `ormlite:"col=rowid"`
}

func (*modelWithoutPK) Table() string { return "" }

type modelWithZeroPK struct {
	ID int `ormlite:"primary,col=rowid"`
}

func (*modelWithZeroPK) Table() string { return "" }

func TestWrongModels(t *testing.T) {
	t.Run("TestDeleteModelWithoutPK", func(t *testing.T) {
		assert.Error(t, Delete(nil, &modelWithoutPK{1}))
	})
	t.Run("TestDeleteModelWithZeroPK", func(t *testing.T) {
		assert.Error(t, Delete(nil, &modelWithZeroPK{}))
	})
}
