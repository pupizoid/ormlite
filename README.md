# ormlite
Simple package that contains some ORM like functionality for `database/sql` especially for sqlite3

[![Build Status](https://travis-ci.org/pupizoid/ormlite.svg?branch=master)](https://travis-ci.org/pupizoid/ormlite)
[![Coverage Status](https://coveralls.io/repos/github/pupizoid/ormlite/badge.svg?branch=master)](https://coveralls.io/github/pupizoid/ormlite?branch=master)
[![GoDoc](https://godoc.org/github.com/pupizoid/ormlite?status.svg)](https://godoc.org/github.com/pupizoid/ormlite)
[![Go Report Card](https://goreportcard.com/badge/github.com/pupizoid/ormlite)](https://goreportcard.com/report/github.com/pupizoid/ormlite)

## Model
```go
type Model interface {
    Table() string
}
```
This package mainly operates with a Model interface. Though there is an ability to load data specifing custom table, Model is used to process relations. 

## CRUD
This package provides a bunch of funcs to allow you create, read, update and delete data.
  
### QueryStruct
Loads data from table and scans it into provided struct. If query was too broad to load more than one rows, the latest of them will be scanned. Also this function supports loading relations which will be described below.

```go
type SimpleStruct struct {
  IntField int `ormlite:"col=rowid,primary"`
  Text string
  UnusedField bool `ormlite:"-"
}

var s SimpleStruct
err := QueryStruct(db, "", nil, &s)
```

Let's describe some tags used in example struct:
- `col` - let you specify custom column name to be scanned to the field
- `primary` - indicates model primary key, it's basicly used when saving model
- `-` - hide field for package so it won't be affected at any kind

### QuerySlice
This is very similar to QueryStruct except that it loads multiple rows in a slice. Also QuerySlice for now does not support loading relations due cyclic dependency.

### Upsert
This function is used to save or update existing model, if model has `primary` field and it's value is zero - this model will be inserted to the model's table. Otherwise model's row will be updated accordint it's current values. This function also supports relations except `hasMany`.
```go
err := Upsert(db, &s)
```
### Delete
This function... yea, it deletes model from database, using all it's fields except relational as identification condition. So if you loaded any model and changed it and then will call Delete nothing will hapen.

## Options

```go
type Options struct {
	// Add where clause to query
	Where         Where    
	Limit         int      
	Offset        int      
	OrderBy       *OrderBy 
	// Load relations to specified depth,
	// if depth is 0 don't load any relations
	RelationDepth int      
}
```

For most queries is't enough to use `DefaultOptions()` which has relation depth equal to 1. 

If you already have variable containing Options, you can extend them with additional settings with following functions:
- WithLimit
- WithOffset
- WithOrder
- WithWhere

## Relations

QueryStruct, QuerySlice and Upsert support loading relations between models, the supported relation types are:
- Has One
- Has Many
- Many To Many

Since you can control depth of loaded relations, there is no need to be afraid of cycle loading. But there are several tags to configure relations.

### Has One

```go
type Model struct {
	Related ormlite.Model `ormlite:"has_one,col=related_model_id"`
}
```

`has_one` indicates that this field represents has one relations type to other model.

`col` is an optional parameter to specify custom column name of foreign id of related model.

### Has Many

```go
type Model struct {
	Related []ormlite.Model `ormlite:"has_many"`
}
```

`has_many` is the only parameter to indicate has many relation, however there is a requirement that related model must have `primary`
 field.
 
 ### Many To Many
 
 ```go
type Model struct {
	Related []ormlite.Model `ormlite:"many_to_many,table=mapping_table,field=model_id"`
}
```

`many_to_many` indicates that field represents many to many relation.

`table` should contain mapping table name to retrieve relation information.

`field` should specify column in mapping table that contains foreign key of original model

Also there is a requirement to related model primary key field to contain `ref` setting that specifies column name of it's foreign key in mapping table.
