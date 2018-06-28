package ormlite

import (
	"database/sql"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type A struct {
	A int    `sqlutils:"col=id"`
	B string `sqlutils:"col=b"`
	C bool   `sqlutils:"col=d"`
}

type testQueryStructSuite struct {
	suite.Suite
	db *sql.DB
}

func (s *testQueryStructSuite) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = db.Exec(`
		create table a (id integer primary key, b text not null, d boolean not null);
		insert into a (b, d) values ('test text', true), ('test text 2', false);

		create table c (name text);
		create table d (name text);
		create table c_to_d(c_id int references c(rowid), d_id int references d(rowid));
		
		insert into c(name) values ('c test 1');
		insert into d(name) values ('d test 1'), ('d test 2'), ('d test 3');

		insert into c_to_d(c_id, d_id) values (1,2), (1,3);
	`)
	require.NoError(s.T(), err)

	s.db = db
}

func (s *testQueryStructSuite) TearDownSuite() {
	s.db.Close()
}

func (s *testQueryStructSuite) TestQueryStruct() {
	var a A
	assert.NoError(s.T(), QueryStruct(s.db, "a", &Options{Where: map[string]interface{}{"id": 1}}, &a))
	assert.Equal(s.T(), "test text", a.B)
	assert.Equal(s.T(), true, a.C)
}

func (s *testQueryStructSuite) TestQueryStructRelations() {
	var c testMtMC
	assert.NoError(s.T(), QueryStruct(
		s.db, "c", &Options{Where: map[string]interface{}{"rowid": 1}, LoadRelations: true}, &c))
	assert.Equal(s.T(), 2, len(c.Ds))
	spew.Dump(c)
}

func (s *testQueryStructSuite) TestQuerySlice() {
	var aa []*A
	assert.NoError(s.T(), QuerySlice(s.db, "a", nil, &aa))
	assert.Equal(s.T(), 2, len(aa))
	assert.Equal(s.T(), "test text 2", aa[1].B)
	assert.Equal(s.T(), false, aa[1].C)
}

func TestQuery(t *testing.T) {
	suite.Run(t, new(testQueryStructSuite))
}

type upsertA struct {
	ID   int `sqlutils:"col=rowid,primary"`
	Name string
}

func (m *upsertA) Table() string { return "a" }

type b struct {
	ID   int `sqlutils:"col=rowid,primary"`
	Name string
	A    *upsertA `sqlutils:"col=a_id,one_to_one"`
}

func (m *b) Table() string { return "b" }

type upsertIfNotExistFixture struct {
	suite.Suite
	db *sql.DB
}

//

type testMtMD struct {
	ID   int `sqlutils:"col=rowid,ref=d_id,primary"`
	Name string
}

func (m *testMtMD) Table() string { return "d" }

type testMtMC struct {
	ID   int `sqlutils:"col=rowid,ref=c_id,primary"`
	Name string
	Ds   []*testMtMD `sqlutils:"many_to_many,table=c_to_d,field=c_id"`
}

func (m *testMtMC) Table() string { return "c" }

func (s *upsertIfNotExistFixture) SetupSuite() {
	c, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = c.Exec(`
	create table a (name text); 
	create table b (name text, a_id int references a(rowid));	

	create table c (name text);
	create table d (name text);
	create table c_to_d(c_id int references c(rowid), d_id int references d(rowid));
	
	insert into c(name) values ('c test 1'), ('c test 2'), ('c test 3');
	insert into d(name) values ('d test 1'), ('d test 2'), ('d test 3');
	`)
	require.NoError(s.T(), err)
	s.db = c
}

func (s *upsertIfNotExistFixture) TearDownSuite() {
	s.db.Close()
}

func (s *upsertIfNotExistFixture) TestUpsertOneToOne() {
	a1 := &upsertA{Name: "A struct"}
	assert.NoError(s.T(), Upsert(s.db, a1))
	assert.Equal(s.T(), 1, a1.ID)
	// test update
	a1.Name = "A struct edited"
	assert.NoError(s.T(), Upsert(s.db, a1))
	// assert
	var test upsertA
	assert.NoError(s.T(), QueryStruct(s.db, "a", &Options{Where: map[string]interface{}{"rowid": 1}}, &test))
	assert.Equal(s.T(), a1.ID, test.ID)
	assert.Equal(s.T(), a1.Name, test.Name)
	// check refecence insert
	b1 := &b{Name: "B struct", A: a1}
	assert.NoError(s.T(), Upsert(s.db, b1))
	assert.Equal(s.T(), 1, b1.ID)
}

func (s *upsertIfNotExistFixture) TestUpsertManyToMany() {
	var (
		dd []*testMtMD
		cc []*testMtMC
	)
	assert.NoError(s.T(), QuerySlice(s.db, "c", nil, &cc))
	assert.NoError(s.T(), QuerySlice(s.db, "d", nil, &dd))
	// test add relations
	cc[0].Ds = append(cc[0].Ds, dd[1:]...)
	assert.NoError(s.T(), Upsert(s.db, cc[0]))
	var c int
	rows, err := s.db.Query("select count(*) from c_to_d")
	assert.NoError(s.T(), err)
	for rows.Next() {
		assert.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 2, c)
	// test remove relation
	cc[0].Ds = cc[0].Ds[:len(cc[0].Ds)-1]
	assert.NoError(s.T(), Upsert(s.db, cc[0]))
	rows, err = s.db.Query("select count(*) from c_to_d")
	assert.NoError(s.T(), err)
	for rows.Next() {
		assert.NoError(s.T(), rows.Scan(&c))
	}
	assert.Equal(s.T(), 1, c)
}

func TestUpsert(t *testing.T) {
	suite.Run(t, new(upsertIfNotExistFixture))
}

type deleteA struct {
	ID int `sqlutils:"primary"`
	B  string
	D  bool
}

func (m *deleteA) Table() string { return "a" }

type testDeleteModel struct {
	suite.Suite
	db *sql.DB
}

func (s *testDeleteModel) SetupSuite() {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(s.T(), err)

	_, err = db.Exec(`
		create table a (id integer primary key, b text not null, d boolean not null);
		insert into a (b, d) values ('test text', true), ('test text 2', false);
	`)
	require.NoError(s.T(), err)

	s.db = db
}

func (s *testDeleteModel) TearDownSuite() {
	s.db.Close()
}

func (s *testDeleteModel) TestDelete() {
	var res1, res2 []*deleteA
	assert.NoError(s.T(), QuerySlice(s.db, "a", nil, &res1))
	assert.Equal(s.T(), 2, len(res1))
	// delete second one
	assert.NoError(s.T(), Delete(s.db, res1[1]))
	// assert now table contains one row
	assert.NoError(s.T(), QuerySlice(s.db, "a", nil, &res2))
	assert.Equal(s.T(), 1, len(res2))
	assert.Equal(s.T(), res1[0], res2[0])
}

func TestDelete(t *testing.T) {
	suite.Run(t, new(testDeleteModel))
}
