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
	ID               int64 `ormlite:"primary"`
	NotTaggedField   string
	TaggedField      string `ormlite:"col=tagged_field"`
	OmittedField     string `ormlite:"-"`
	notExportedField string
}

func (*simpleModel) Table() string { return "simple_model" }

var _ Model = (*simpleModel)(nil)

type simpleModelWithRelation struct {
	ID             int64 `ormlite:"primary"`
	NotTaggedField string
	Related        *simpleModel `ormlite:"has_one,col=related_id"`
}

func (sm *simpleModelWithRelation) Table() string { return "simple_model_has_one" }

type simpleModelWithCycleRelation struct {
	ID             int64 `ormlite:"primary"`
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
						id integer primary key,
                        not_tagged_field text,
                        tagged_field text
				);
				
				create table simple_model_has_one (
					id integer primary key,
					not_tagged_field text,
					related_id int
				);

				create table simple_model_has_one_cycle (
					id integer primary key,
					not_tagged_field text,
					related_id int
				);

                insert into simple_model(not_tagged_field, tagged_field) values
					('test', 'test tagged'),
					('asdad', 'assddffgh'),
					('1111', '22222');
						
				insert into simple_model_has_one(not_tagged_field, related_id) values 
					('test', null),
					('test 2 ', 1),
					('test 3 ', 2);

				insert into simple_model_has_one_cycle(not_tagged_field, related_id) values 
					('test', 1);
        `)
	assert.NoError(s.T(), err)
	s.db = c
}

func (s *simpleModelFixture) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
}

func (s *simpleModelFixture) TestCount() {
	count, err := Count(s.db, &simpleModel{}, nil)
	if assert.NoError(s.T(), err) {
		assert.EqualValues(s.T(), 3, count)
	}

	count, err = Count(s.db, &simpleModel{}, &Options{Where: Where{"id": 1}})
	if assert.NoError(s.T(), err) {
		assert.EqualValues(s.T(), 1, count)
	}

	count, err = Count(s.db, &simpleModel{}, &Options{Where: Where{"tagged_field": "22"}})
	if assert.NoError(s.T(), err) {
		assert.EqualValues(s.T(), 1, count)
	}
}

func (s *simpleModelFixture) TestSearchLike() {
	var m simpleModel
	if assert.NoError(s.T(), QueryStruct(s.db, &Options{Where: Where{"tagged_field": "2"}}, &m)) {
		assert.EqualValues(s.T(), 3, m.ID)
	}
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

	_, err := Delete(s.db, &m2)
	assert.NoError(s.T(), err)

	var m3 = simpleModelWithRelation{NotTaggedField: "some not tagged field"}
	assert.NoError(s.T(), Upsert(s.db, &m3))
	// test foreign key is set to null
	var m4 simpleModelWithRelation
	assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": m3.ID}), &m4))
	assert.Nil(s.T(), m4.Related)
}

func (s *simpleModelFixture) TestQuerySlice() {
	var mm []*simpleModel
	assert.NoError(s.T(), QuerySlice(s.db, nil, &mm))
	assert.NotEmpty(s.T(), mm)
}

func (s *simpleModelFixture) TestQuerySliceWithRelations() {
	var mm []*simpleModelWithRelation
	assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm))
	if assert.NotEmpty(s.T(), mm) {
		assert.Nil(s.T(), mm[0].Related)
		if assert.NotNil(s.T(), mm[1].Related) {
			assert.Equal(s.T(), mm[1].Related.TaggedField, "test tagged")
		}
		if assert.NotNil(s.T(), mm[2].Related) {
			assert.Equal(s.T(), mm[2].Related.TaggedField, "assddffgh")
		}
	}

}

func (s *simpleModelFixture) TestQuerySliceWithCycleRelation() {
	var mm []*simpleModelWithCycleRelation
	assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm))
	if assert.NotEmpty(s.T(), mm) {
		if assert.NotNil(s.T(), mm[0].Related) {
			assert.Nil(s.T(), mm[0].Related.Related)
		}
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
		assert.NotEqual(s.T(), int64(1), m.ID, "First row shouldn't be returned since offset")
	}
}

func (s *simpleModelFixture) TestOrderBy() {
	var mm []*simpleModel
	require.NoError(s.T(), QuerySlice(s.db, WithOrder(DefaultOptions(), OrderBy{Field: "rowid", Order: "desc"}), &mm))
	assert.NotEmpty(s.T(), mm)
	assert.NotEqual(s.T(), int64(1), mm[0].ID)
}

func (s *simpleModelFixture) TestOrderByWithLimit() {
	var mm []*simpleModel
	require.NoError(s.T(), QuerySlice(s.db, &Options{Limit: 2, OrderBy: &OrderBy{Field: "rowid", Order: "desc"}}, &mm))
	assert.NotEmpty(s.T(), mm)
	assert.NotEqual(s.T(), int64(2), mm[0].ID)
}

func TestSimpleModel(t *testing.T) {
	suite.Run(t, new(simpleModelFixture))
}

type modelWithCompoundPrimaryKey struct {
	FirstID  int64 `ormlite:"primary,col=first_id,ref=first_id_ref"`
	SecondID int64 `ormlite:"primary,col=second_id,ref=second_id_ref"`
	Field    string
}

func (s *modelWithCompoundPrimaryKey) Table() string { return "model_with_compound_primary_key" }

type modelWithCompoundPrimaryKeyFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *modelWithCompoundPrimaryKeyFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	assert.NoError(s.T(), err)

	_, err = c.Exec(`
				create table model_with_compound_primary_key (
					first_id integer not null,
					second_id integer not null,
					field text,
					primary key(first_id, second_id)
				);
	`)
	assert.NoError(s.T(), err)
	s.db = c
}

func (s *modelWithCompoundPrimaryKeyFixture) TestACreate() {
	cases := []modelWithCompoundPrimaryKey{
		{1, 2, "1"},
		{1, 1, "2"},
		{2, 1, "3"},
	}
	for _, model := range cases {
		assert.NoError(s.T(), Upsert(s.db, &model))
	}
}

func (s *modelWithCompoundPrimaryKeyFixture) TestBRead() {
	var m modelWithCompoundPrimaryKey
	assert.NoError(s.T(), QueryStruct(s.db, &Options{Where: Where{"first_id": 1, "second_id": 1}, Divider: AND}, &m))
	assert.Equal(s.T(), "2", m.Field)
	var mm []*modelWithCompoundPrimaryKey
	assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm))
	assert.Equal(s.T(), 3, len(mm))
}

func (s *modelWithCompoundPrimaryKeyFixture) TestCUpdate() {
	assert.NoError(s.T(), Upsert(s.db, &modelWithCompoundPrimaryKey{1, 1, "4"}))
	var m modelWithCompoundPrimaryKey
	if assert.NoError(s.T(), QueryStruct(s.db, &Options{Where: Where{"first_id": 1, "second_id": 1}, Divider: AND}, &m)) {
		assert.Equal(s.T(), "4", m.Field)
	}

}

func (s *modelWithCompoundPrimaryKeyFixture) TestDelete() {
	_, err := Delete(s.db, &modelWithCompoundPrimaryKey{1, 1, ""})
	assert.NoError(s.T(), err)
	var mm []*modelWithCompoundPrimaryKey
	if assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm)) {
		assert.Equal(s.T(), 2, len(mm))
	}
}

func TestModelWithCompoundPrimaryKey(t *testing.T) {
	suite.Run(t, new(modelWithCompoundPrimaryKeyFixture))
}

type relatedModel struct {
	ID    int64 `ormlite:"col=rowid,primary,ref=rel_id"`
	Field string
}

func (m *relatedModel) Table() string { return "related_model" }

type relatedModelWithID struct {
	ID    int64 `ormlite:"primary,ref=whatever"`
	Field string
}

func (*relatedModelWithID) Table() string { return "related_model_with_id" }

type modelHasOne struct {
	ID      int64         `ormlite:"col=rowid,primary"`
	Related *relatedModel `ormlite:"has_one,col=rel_id"`
}

func (m *modelHasOne) Table() string { return "one_to_one_rel" }

type modelHasOneCycle struct {
	ID      int64             `ormlite:"col=rowid,primary"`
	Related *modelHasOneCycle `ormlite:"has_one,col=rel_id"`
}

func (m *modelHasOneCycle) Table() string { return "one_to_one_cycle_rel" }

type modelHasOneWithIDAndRef struct {
	ID      int64               `ormlite:"col=rowid,primary"`
	Related *relatedModelWithID `ormlite:"has_one,col=rel_id"`
}

func (*modelHasOneWithIDAndRef) Table() string { return "one_to_one_with_id_rel" }

type hasOneRelationFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *hasOneRelationFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
                create table related_model ( field text );
				create table related_model_with_id (id integer primary key, field text);
                create table one_to_one_rel ( rel_id int );
				create table one_to_one_cycle_rel (rel_id int);
				create table one_to_one_with_id_rel (rel_id int);

                insert into related_model (field) values('test'), ('test 2');
                insert into related_model_with_id (field) values('id 1'), ('id 2');
                insert into one_to_one_rel (rel_id) values(1), (null);
				insert into one_to_one_cycle_rel (rel_id) values (1);
				insert into one_to_one_with_id_rel (rel_id) values (2);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *hasOneRelationFixture) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
}

func (s *hasOneRelationFixture) TestQueryStruct() {
	var m modelHasOne
	require.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": 1}), &m))
	assert.Equal(s.T(), int64(1), m.ID)
	require.NotNil(s.T(), m.Related)
	assert.Equal(s.T(), "test", m.Related.Field)
	assert.Equal(s.T(), int64(1), m.Related.ID)
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
	assert.Equal(s.T(), int64(2), mm[2].Related.ID)
	assert.Equal(s.T(), "test 2", mm[2].Related.Field)
	//
	_, err := Delete(s.db, mm[0])
	assert.NoError(s.T(), err)
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

func (s *hasOneRelationFixture) TestWithIDRelatedModel() {
	var m modelHasOneWithIDAndRef
	assert.NoError(s.T(), QueryStructContext(
		context.Background(), s.db, &Options{Where: Where{"rowid": 1}, RelationDepth: 1}, &m))
	assert.NotNil(s.T(), m.Related)
}

func TestHasOneRelation(t *testing.T) {
	suite.Run(t, new(hasOneRelationFixture))
}

type relatingModel struct {
	ID      int64         `ormlite:"col=rowid,primary"`
	Related *hasManyModel `ormlite:"has_one,col=related_id"`
}

func (*relatingModel) Table() string { return "relating_model" }

type hasManyModel struct {
	ID      int64 `ormlite:"col=rowid,primary"`
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
	require.NoError(s.T(), s.db.Close())
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
	ID    int64 `ormlite:"primary,ref=c_rel_id"`
	Field string
}

func (*relatingModelWithCustomPK) Table() string { return "relating_model_custom_pk" }

type modelManyToMany struct {
	ID      int64 `ormlite:"col=rowid,primary,ref=m_id"`
	Name    string
	Related []*relatedModel `ormlite:"many_to_many,table=mtm,field=m_id"`
}

func (*modelManyToMany) Table() string { return "mtm_model" }

type modelManyToManyWithCondition struct {
	ID           int64 `ormlite:"col=rowid,primary,ref=m_id"`
	Name         string
	RelatedFalse []*relatedModel `ormlite:"many_to_many,table=mtm_with_condition,field=m_id,condition:value=0"`
	RelatedTrue  []*relatedModel `ormlite:"many_to_many,table=mtm_with_condition,field=m_id,condition:value=1"`
}

func (*modelManyToManyWithCondition) Table() string { return "mtm_model" }

type modelManyToManyWithCustomPK struct {
	ID      int64 `ormlite:"col=rowid,primary,ref=m_id"`
	Name    string
	Related []*relatingModelWithCustomPK `ormlite:"many_to_many,table=mtm_with_custom_model,field=m_id"`
}

func (*modelManyToManyWithCustomPK) Table() string { return "mtm_model" }

type modelManyToManyWithCompoundPK struct {
	ID      int64 `ormlite:"primary,ref=model_id"`
	Name    string
	Related []*modelWithCompoundPrimaryKey `ormlite:"many_to_many,table=mtm_with_compound_pk,field=model_id"`
}

func (m *modelManyToManyWithCompoundPK) Table() string { return "mtm_model_with_id" }

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
				--
				create table mtm_with_condition ( m_id int, rel_id int, value boolean not null );
				insert into mtm_with_condition (m_id, rel_id, value) values (1,1,true), (1,3,true), (1,1,false), (1,2,true);

				create table relating_model_custom_pk(id integer primary key, field text);
				insert into relating_model_custom_pk(field) values ('common test 1'), ('common test 2');
				create table mtm_with_custom_model (m_id int, c_rel_id int);
				insert into mtm_with_custom_model(m_id, c_rel_id) values (1,1), (1,2);
				-- mtm to compound key
				create table mtm_model_with_id(id integer primary key, name text);
				create table mtm_with_compound_pk(model_id int, first_id_ref int, second_id_ref int);
				create table model_with_compound_primary_key (
					first_id integer not null,
					second_id integer not null,
					field text,
					primary key(first_id, second_id)
				);
				insert into model_with_compound_primary_key(first_id, second_id, field) values (1,1,'1'), (1,2,'2'), (2,1,'3');
				-- mtm to compound with relation
				
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *manyToManyRelationFixture) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
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
		assert.EqualValues(s.T(), modelManyToManyWithCondition{
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
	require.NoError(s.T(), QuerySliceContext(context.Background(), s.db, WithLimit(DefaultOptions(), 100), &mm))
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
	// test query with limit
	var mmccc []*modelManyToManyWithCustomPK
	if assert.NoError(s.T(), QuerySliceContext(context.Background(), s.db, WithLimit(DefaultOptions(), 1), &mmccc)) {
		for _, m := range mmccc {
			assert.Equal(s.T(), &modelManyToManyWithCustomPK{
				ID: 1, Name: "name",
				Related: []*relatingModelWithCustomPK{{1, "common test 1"}},
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
	assert.NoError(s.T(), Upsert(s.db, &m2))
	var c int
	rows, err := s.db.Query("select count(*) from mtm")
	require.NoError(s.T(), err)
	for rows.Next() {
		require.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 3, c)

	_, err = Delete(s.db, &m2)
	assert.NoError(s.T(), err)
	// insert new model
	var m3 = modelManyToMany{
		Name:    "new model",
		Related: []*relatedModel{{ID: 3}},
	}
	assert.NoError(s.T(), Upsert(s.db, &m3))
	assert.Equal(s.T(), int64(2), m3.ID)
	// check insert with condition
	var mc = modelManyToManyWithCondition{
		ID:           1,
		Name:         "name",
		RelatedFalse: []*relatedModel{{1, "test 1"}, {3, "test 3"}},
		RelatedTrue:  []*relatedModel{{2, "test 2"}, {3, "test 3"}},
	}
	assert.NoError(s.T(), Upsert(s.db, &mc))
	var mc1 modelManyToManyWithCondition
	if assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": mc.ID}), &mc1)) {
		assert.Equal(s.T(), mc, mc1)
	}
	// test Update
	if assert.NoError(s.T(), Update(s.db, &modelManyToManyWithCondition{ID: 1, Name: "new"})) {
		var mc2 modelManyToManyWithCondition
		if assert.NoError(s.T(), QueryStruct(s.db, WithWhere(DefaultOptions(), Where{"rowid": mc.ID}), &mc2)) {
			assert.Equal(s.T(), "new", mc2.Name)
			assert.Equal(s.T(), []*relatedModel{{1, "test 1"}, {3, "test 3"}}, mc2.RelatedFalse)
		}
	}
}

func (s *manyToManyRelationFixture) TestCompoundKeys() {
	// test creation
	assert.NoError(s.T(), Upsert(s.db, &modelManyToManyWithCompoundPK{
		Name:    "test",
		Related: []*modelWithCompoundPrimaryKey{{1, 2, "2"}, {1, 1, "1"}},
	}))
	// test fetching
	var m modelManyToManyWithCompoundPK
	if assert.NoError(s.T(), QueryStructContext(
		context.Background(), s.db, &Options{Where: Where{"id": 1}, RelationDepth: 1}, &m)) {
		assert.NotZero(s.T(), m.ID)
		assert.Equal(s.T(), 2, len(m.Related))
	}
	var mm []*modelManyToManyWithCompoundPK
	if assert.NoError(s.T(), QuerySliceContext(context.Background(), s.db, DefaultOptions(), &mm)) {
		assert.NotNil(s.T(), mm)
		assert.Equal(s.T(), 2, len(m.Related))
	}
	// test update
	m.Name = "test edited"
	m.Related = []*modelWithCompoundPrimaryKey{{1, 2, "2"}, {2, 1, "3"}}
	if assert.NoError(s.T(), Upsert(s.db, &m)) {
		var mNew modelManyToManyWithCompoundPK
		if assert.NoError(s.T(), QueryStructContext(
			context.Background(), s.db, &Options{Where: Where{"id": 1}, RelationDepth: 1}, &mNew)) {
			assert.Equal(s.T(), m, mNew)
		}
	}
	// test delete
	_, err := Delete(s.db, &m)
	assert.NoError(s.T(), err)
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
                create table one(rel_id int);
                create table two(rel_id int);

                insert into related_model(field) values('1'), ('2'), ('3'), ('4'), ('5');
                insert into one(rel_id) values (1), (2), (3);
                insert into two(rel_id) values (4), (5);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *modelMultiTableFixture) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
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
	assert.NoError(s.T(), Upsert(s.db, &m))
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
	_, err := Delete(s.db, new(modelMultiTable))
	assert.Error(s.T(), err)
}

func TestMultiTableModel(t *testing.T) {
	suite.Run(t, new(modelMultiTableFixture))
}

type modelWithoutPK struct {
	ID int64 `ormlite:"col=rowid"`
}

func (*modelWithoutPK) Table() string { return "" }

type modelWithZeroPK struct {
	ID int64 `ormlite:"primary,col=rowid"`
}

func (*modelWithZeroPK) Table() string { return "" }

func TestWrongModels(t *testing.T) {
	t.Run("TestDeleteModelWithoutPK", func(t *testing.T) {
		_, err := Delete(nil, &modelWithoutPK{1})
		assert.Error(t, err)
	})
	t.Run("TestDeleteModelWithZeroPK", func(t *testing.T) {
		_, err := Delete(nil, &modelWithZeroPK{})
		assert.Error(t, err)
	})
}

type relatedModelFK struct {
	ID    int64 `ormlite:"primary,ref=related_id"`
	Field string
}

func (*relatedModelFK) Table() string { return "related_model" }

type modelWithCompoundWithForeign struct {
	FirstID int64           `ormlite:"primary,col=first_id,ref=first_id"`
	Related *relatedModelFK `ormlite:"primary,col=second_id,ref=second_id,has_one"`
	Name    string
}

func (*modelWithCompoundWithForeign) Table() string { return "complex_model" }

type modelManyToManyWithCompoundWithForeign struct {
	ID      int64 `ormlite:"primary,ref=m_id"`
	Name    string
	Related []*modelWithCompoundWithForeign `ormlite:"many_to_many,table=mapping,field=m_id"`
}

func (*modelManyToManyWithCompoundWithForeign) Table() string { return "model" }

type mtmCompoundKeyAsHasOneRelationFixture struct {
	suite.Suite
	db *sql.DB
}

func (s *mtmCompoundKeyAsHasOneRelationFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:?_fk=1")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
                create table related_model(id integer primary key, field text);
                insert into related_model(field) values('1'), ('2'), ('3'), ('4'), ('5');
                
				create table complex_model(
					first_id integer primary key, 
					second_id integer references related_model (id),
					name text
				);
				insert into complex_model(second_id, name) values (1, '1'), (2, '2'), (3, '3');		

				create table model(id integer primary key, name text);
				insert into model(name) values ('1'), ('2'), ('3');

				create table mapping(
					m_id integer references model (id) on delete cascade, 
					first_id integer,
					second_id integer
-- 					foreign key (first_id, second_id) references complex_model(first_id, second_id)
				);
				insert into mapping(m_id, first_id, second_id) values (1, 1, 1), (1, 3, 3);
        `)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *mtmCompoundKeyAsHasOneRelationFixture) Test() {
	// test create
	assert.NoError(s.T(), Upsert(s.db, &modelManyToManyWithCompoundWithForeign{
		Name: "4",
		Related: []*modelWithCompoundWithForeign{
			{FirstID: 2, Related: &relatedModelFK{2, "2"}, Name: "2"},
		},
	}))
	// test fetch
	var m modelManyToManyWithCompoundWithForeign
	assert.NoError(s.T(), QueryStructContext(
		context.Background(), s.db, &Options{Where: Where{"id": 1}, RelationDepth: 2}, &m))
	if assert.NotNil(s.T(), m.Related, "Relations were not loaded") {
		assert.Equal(s.T(), 2, len(m.Related))
	}
	var mm []*modelManyToManyWithCompoundWithForeign
	assert.NoError(s.T(), QuerySliceContext(
		context.Background(), s.db, &Options{Where: Where{"id": 4}, RelationDepth: 2}, &mm))
	if assert.NotNil(s.T(), mm) && assert.Equal(s.T(), 1, len(mm)) {
		if assert.NotNil(s.T(), mm[0].Related) {
			assert.Equal(s.T(), 1, len(mm[0].Related))
		}
	}
	// test update
	m.Related = []*modelWithCompoundWithForeign{
		{FirstID: 3, Related: &relatedModelFK{3, "3"}, Name: "3"},
	}
	assert.NoError(s.T(), Upsert(s.db, &m))
	var mNew modelManyToManyWithCompoundWithForeign
	if assert.NoError(s.T(), QueryStructContext(
		context.Background(), s.db, &Options{Where: Where{"id": 1}, RelationDepth: 2}, &mNew)) {
		if assert.NotNil(s.T(), mNew.Related) {
			assert.Equal(s.T(), 1, len(mNew.Related))
			assert.Equal(s.T(), int64(3), mNew.Related[0].FirstID)
		}
	}
	// delete
	_, err := Delete(s.db, &m)
	assert.NoError(s.T(), err)
}

func TestCompoundKeyAsHasOneRelation(t *testing.T) {
	suite.Run(t, new(mtmCompoundKeyAsHasOneRelationFixture))
}

type testSearchByRelatedSuite struct {
	suite.Suite
	db *sql.DB
}

type testSearchBaseModel struct {
	ID         int64 `ormlite:"primary,ref=base_id"`
	Name       string
	HasOne     *testSearchHasOneModel    `ormlite:"has_one,col=has_one"`
	HasMany    []*testSearchHasManyModel `ormlite:"has_many"`
	ManyToMany []*testSearchMTMModel     `ormlite:"many_to_many,table=relation_table,field=base_id"`
}

func (*testSearchBaseModel) Table() string { return "base_model" }

type testSearchHasOneModel struct {
	ID   int64 `ormlite:"primary,ref=some_id"`
	Name string
}

func (*testSearchHasOneModel) Table() string { return "has_one_model" }

type testSearchMTMModel struct {
	ID   int64 `ormlite:"primary,ref=mtm_id"`
	Name string
}

func (*testSearchMTMModel) Table() string { return "mtm_model" }

type testSearchHasManyModel struct {
	ID         int64                `ormlite:"primary"`
	BaseModel1 *testSearchBaseModel `ormlite:"has_one,col=bm1"`
	BaseModel2 *testSearchBaseModel `ormlite:"has_one,col=bm2"`
}

func (*testSearchHasManyModel) Table() string { return "has_many_model" }

func (s *testSearchByRelatedSuite) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:?_fk=1")
	require.NoError(s.T(), err)

	_, err = db.Exec(`
		create table base_model(id integer primary key, name text, has_one integer);
		create table has_one_model(id integer primary key, name text);
		create table has_many_model(id integer primary key, bm1 integer, bm2 integer);
		create table mtm_model(id integer primary key, name text);
		create table relation_table(base_id integer, mtm_id integer); 
	`)
	require.NoError(s.T(), err)
	s.db = db

	var hasOneModel = testSearchHasOneModel{Name: "has one"}

	require.NoError(s.T(), Insert(db, &testSearchMTMModel{Name: "test"}))
	require.NoError(s.T(), Insert(db, &hasOneModel))
	err = Upsert(db, &testSearchHasManyModel{
		BaseModel1: &testSearchBaseModel{ID: 1},
		BaseModel2: &testSearchBaseModel{ID: 1},
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), Upsert(db, &testSearchHasManyModel{
		BaseModel1: &testSearchBaseModel{ID: 1},
		BaseModel2: &testSearchBaseModel{ID: 2},
	}))
	require.NoError(s.T(), Insert(db, &testSearchMTMModel{Name: "mtm 1"}))
	require.NoError(s.T(), Insert(db, &testSearchMTMModel{Name: "mtm 2"}))
	require.NoError(s.T(), Insert(db, &testSearchMTMModel{Name: "mtm 3"}))

	require.NoError(s.T(), Upsert(db, &testSearchBaseModel{
		Name: "Test 1", HasOne: &hasOneModel,
		ManyToMany: []*testSearchMTMModel{{ID: 1}, {ID: 2}, {ID: 3}},
	}))

	require.NoError(s.T(), Upsert(db, &testSearchBaseModel{
		Name: "Test 2", HasOne: &hasOneModel,
		ManyToMany: []*testSearchMTMModel{{ID: 1}},
	}))

	require.NoError(s.T(), Upsert(db, &testSearchBaseModel{
		Name: "Test 3", HasOne: &hasOneModel,
	}))
}

func (s testSearchByRelatedSuite) TestScheme() {
	var mm []*testSearchBaseModel
	if assert.NoError(s.T(), QuerySlice(s.db, DefaultOptions(), &mm)) {
		assert.NotNil(s.T(), mm)
		assert.Len(s.T(), mm, 3)
	}
}

func (s *testSearchByRelatedSuite) TestSearchByHasMany() {
	var mm []*testSearchBaseModel
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 1}}}, &mm)) {
		assert.Len(s.T(), mm, 1)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 2}}}, &mm)) {
		assert.Len(s.T(), mm, 2)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 3}}}, &mm)) {
		assert.Len(s.T(), mm, 0)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 2}}, Where: Where{"name": "Test 1"}, Divider: AND}, &mm)) {
		assert.Len(s.T(), mm, 1)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 1}}, Where: Where{"name": "Test 2"}, Divider: AND}, &mm)) {
		assert.Len(s.T(), mm, 0)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 2}}, Limit: 1}, &mm)) {
		assert.Len(s.T(), mm, 1)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchHasManyModel{ID: 2}, &testSearchHasManyModel{ID: 1}}}, &mm)) {
		assert.Len(s.T(), mm, 2)
	}
}

func (s *testSearchByRelatedSuite) TestSearchByManyToMany() {
	var mm []*testSearchBaseModel
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchMTMModel{ID: 1}}}, &mm)) {
		assert.Len(s.T(), mm, 2)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchMTMModel{ID: 2}}}, &mm)) {
		assert.Len(s.T(), mm, 1)
	}
	mm = nil
	if assert.NoError(s.T(), QuerySlice(s.db, &Options{RelatedTo: []IModel{&testSearchMTMModel{}}}, &mm)) {
		assert.Len(s.T(), mm, 1)
	}

	count, err := Count(s.db, &testSearchBaseModel{}, &Options{RelatedTo: []IModel{&testSearchMTMModel{ID: 2}}})
	if assert.NoError(s.T(), err) {
		assert.EqualValues(s.T(), 1, count)
	}
}

func TestSearchByRelated(t *testing.T) {
	suite.Run(t, new(testSearchByRelatedSuite))
}

type MTMModel struct {
	ID       int64 `ormlite:"primary,ref=model_id"`
	Name     string
	Children []*MTMModel `ormlite:"many_to_many,table=rel_table,field=parent_id"`
}

func (*MTMModel) Table() string { return "mtm_model" }

type testCustomFieldInMTMModel struct {
	suite.Suite
	db *sql.DB
}

func (s *testCustomFieldInMTMModel) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = db.Exec(`
		create table mtm_model (id integer primary key, name text);
		create table rel_table (parent_id integer, model_id integer);
		
		insert into mtm_model(name) values ('tst 1'), ('test 2'), ('test 3'), ('test 4');
		insert into rel_table (parent_id, model_id) values (1,2), (1,3), (4,1);
	`)
	require.NoError(s.T(), err)
	s.db = db
}

func (s *testCustomFieldInMTMModel) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
}

func (s *testCustomFieldInMTMModel) TestParenthesis() {
	var mm []*MTMModel
	require.NoError(s.T(), QuerySlice(s.db, &Options{RelationDepth: 1, Where: Where{"id": 1}}, &mm))

	if assert.Len(s.T(), mm, 1) {
		assert.Len(s.T(), mm[0].Children, 2)
	}

	mm = nil
	require.NoError(s.T(), QuerySlice(s.db, &Options{RelationDepth: 1, Where: Where{"id": 4}}, &mm))
	if assert.Len(s.T(), mm, 1) {
		assert.Len(s.T(), mm[0].Children, 1)
	}
}

func TestCustomFieldInMTM(t *testing.T) {
	suite.Run(t, new(testCustomFieldInMTMModel))
}

type testOperatorsModel struct {
	ID     int64 `ormlite:"primary"`
	Number int
}

func (m *testOperatorsModel) Table() string { return "test" }

func TestGreaterOrLessOperator(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_fk=1")
	require.NoError(t, err)

	_, err = db.Exec(`
		create table test(id integer primary key, number integer);
		insert into test(number) values (1), (2), (3), (4), (5);
	`)
	require.NoError(t, err)

	var mm []*testOperatorsModel
	if assert.NoError(t, QuerySlice(db, &Options{Where: Where{"number": Greater(4)}}, &mm)) {
		if assert.Len(t, mm, 1) {
			assert.EqualValues(t, 5, mm[0].ID)
		}
	}

	mm = nil
	if assert.NoError(t, QuerySlice(db, &Options{Where: Where{"number": Less(3)}}, &mm)) {
		assert.Len(t, mm, 2)
	}

	mm = nil
	if assert.NoError(t, QuerySlice(db, &Options{Where: Where{"number": GreaterOrEqual(3)}}, &mm)) {
		assert.Len(t, mm, 3)
	}

	mm = nil
	if assert.NoError(t, QuerySlice(db, &Options{Where: Where{"number": LessOrEqual(2)}}, &mm)) {
		assert.Len(t, mm, 2)
	}

	mm = nil
	if assert.NoError(t, QuerySlice(db, &Options{Where: Where{"number": NotEqual(3)}}, &mm)) {
		assert.Len(t, mm, 4)
	}
}

func TestBitwiseQuerying(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_fk=1")
	require.NoError(t, err)

	_, err = db.Exec(`
		create table test(id integer primary key, number integer);
		insert into test(number) values (108);
	`)
	require.NoError(t, err)

	var m testOperatorsModel
	if assert.NoError(t, QueryStruct(db, &Options{Where: Where{"number": BitwiseAND(30)}}, &m)) {
		assert.EqualValues(t, 1, m.ID)
	}

	var m1 testOperatorsModel
	if assert.NoError(t, QueryStruct(db, &Options{Where: Where{"number": BitwiseANDStrict(30)}}, &m1)) {
		assert.EqualValues(t, 0, m1.ID)
	}

	var m2 testOperatorsModel
	if assert.NoError(t, QueryStruct(db, &Options{Where: Where{"number": BitwiseANDStrict(40)}}, &m2)) {
		assert.EqualValues(t, 1, m2.ID)
	}

	count, err := Count(db, &testOperatorsModel{}, &Options{Where: Where{"number": BitwiseAND(30)}})
	if assert.NoError(t, err) {
		assert.EqualValues(t, 1, count)
	}

	count, err = Count(db, &testOperatorsModel{}, &Options{Where: Where{"number": BitwiseANDStrict(30)}})
	if assert.NoError(t, err) {
		assert.EqualValues(t, 0, count)
	}

	count, err = Count(db, &testOperatorsModel{}, &Options{Where: Where{"number": BitwiseANDStrict(40)}})
	if assert.NoError(t, err) {
		assert.EqualValues(t, 1, count)
	}
}

type testStrictStringQueryingModel struct {
	ID   int64 `ormlite:"primary"`
	Name string
}

func (*testStrictStringQueryingModel) Table() string { return "test" }

func TestStrictStringQuerying(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:?_fk=1")
	require.NoError(t, err)

	_, err = db.Exec(`
		create table test(id integer primary key, name text);
		insert into test(name) values ('support');
		insert into test(name) values ('subsupport');
	`)
	require.NoError(t, err)

	var m testStrictStringQueryingModel
	if assert.NoError(t, QueryStruct(db, &Options{Where: Where{"name": "support"}}, &m)) {
		assert.EqualValues(t, "subsupport", m.Name)
	}

	var m1 testStrictStringQueryingModel
	if assert.NoError(t, QueryStruct(db, &Options{Where: Where{"name": StrictString("support")}}, &m1)) {
		assert.EqualValues(t, "support", m1.Name)
	}

	count, err := Count(db, &testStrictStringQueryingModel{}, &Options{Where: Where{"name": "support"}})
	if assert.NoError(t, err) {
		assert.EqualValues(t, 2, count)
	}

	count, err = Count(db, &testStrictStringQueryingModel{}, &Options{Where: Where{"name": StrictString("support")}})
	if assert.NoError(t, err) {
		assert.EqualValues(t, 1, count)
	}
}

type testQuerySliceCountModel struct {
	ID   int64 `ormlite:"primary"`
	Attr int
}

func (*testQuerySliceCountModel) Table() string { return "test" }

func TestQuerySliceCount(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	_, err = db.Exec(`
		create table test(id integer primary key , attr int);
		insert into test(attr) values (1);
		insert into test(attr) values (1);
		insert into test(attr) values (1);
		insert into test(attr) values (1);
		insert into test(attr) values (2);
		insert into test(attr) values (2);
		insert into test(attr) values (2);
		insert into test(attr) values (2);
		insert into test(attr) values (2);
		insert into test(attr) values (2);
	`)
	require.NoError(t, err)

	var m []*testQuerySliceCountModel
	var count int
	if assert.NoError(t, QuerySliceCount(db, &Options{Where: Where{"attr": 1}}, &m, &count)) {
		assert.Len(t, m, 4)
		assert.EqualValues(t, 4, count)
		assert.EqualValues(t, 4, m[3].ID)
	}
}

type SelectedColumnsSuite struct {
	suite.Suite
	db *sql.DB
}

type BigModel struct {
	ID      int           `ormlite:"primary"`
	Attr1   int           `ormlite:"col=attr1"`
	Attr2   int           `ormlite:"col=attr2"`
	Attr3   string        `ormlite:"col=attr3"`
	Attr4   float64       `ormlite:"col=attr4"`
	Related *relatedModel `ormlite:"has_one,col=rel_id"`
}

func (b BigModel) Table() string { return "big_model" }

func (s *SelectedColumnsSuite) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = db.Exec("create table big_model(id integer primary key, attr1 int, attr2 int, attr3 string, attr4 float, rel_id int);" +
		"create table related_model(id integer primary key, field text);")
	require.NoError(s.T(), err)
	s.db = db

	require.NoError(s.T(), Insert(db, &BigModel{Attr1: 1, Attr2: 2, Attr3: "first", Attr4: 1.0}))
	require.NoError(s.T(), Insert(db, &BigModel{Attr1: 3, Attr2: 4, Attr3: "second", Attr4: 2.0}))
	require.NoError(s.T(), Insert(db, &BigModel{Attr1: 5, Attr2: 6, Attr3: "third", Attr4: 3.0}))
	require.NoError(s.T(), Insert(db, &BigModel{Attr1: 7, Attr2: 8, Attr3: "forth", Attr4: 4.0}))
	require.NoError(s.T(), Insert(db, &BigModel{Attr1: 9, Attr2: 10, Attr3: "fifth", Attr4: 5.0}))

	require.NoError(s.T(), Upsert(db, &BigModel{
		Attr1:   11,
		Attr2:   11,
		Attr3:   "11",
		Attr4:   11,
		Related: &relatedModel{Field: "Hello"},
	}))
}

func (s *SelectedColumnsSuite) TearDownSuite() {
	require.NoError(s.T(), s.db.Close())
}

func (s *SelectedColumnsSuite) TestQueryStruct() {
	var m BigModel
	require.NoError(s.T(), QueryStruct(s.db, &Options{Columns: map[string]struct{}{
		"attr1": {},
		"attr3": {},
	}, Where: Where{"id": 1}}, &m))

	assert.EqualValues(s.T(), 1, m.ID)
	assert.EqualValues(s.T(), 1, m.Attr1)
	assert.EqualValues(s.T(), 0, m.Attr2)
	assert.EqualValues(s.T(), "first", m.Attr3)
	assert.EqualValues(s.T(), 0.0, m.Attr4)
}

func (s *SelectedColumnsSuite) TestQuerySlice() {
	var mm []*BigModel
	require.NoError(s.T(), QuerySlice(s.db, &Options{Columns: map[string]struct{}{
		"attr2": {},
		"attr4": {},
	}}, &mm))

	if assert.NotNil(s.T(), mm) {
		assert.Len(s.T(), mm, 6)
		assert.EqualValues(s.T(), 6, mm[2].Attr2)
		assert.EqualValues(s.T(), "", mm[2].Attr3)
		assert.EqualValues(s.T(), 3.0, mm[2].Attr4)
		assert.EqualValues(s.T(), (*relatedModel)(nil), mm[5].Related)
	}

}

func TestSelectedColumns(t *testing.T) {
	suite.Run(t, new(SelectedColumnsSuite))
}
