# ormlite
Simple package that contains some ORM like functionality for `database/sql` especially for sqlite3

[![Build Status](https://travis-ci.org/pupizoid/ormlite.svg?branch=master)](https://travis-ci.org/pupizoid/ormlite)
[![codecov](https://codecov.io/gh/pupizoid/ormlite/branch/master/graph/badge.svg)](https://codecov.io/gh/pupizoid/ormlite)
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
- `-` - hiddens field for package so it won't be affected at any kind

### QuerySlice
This is very similar to QueryStruct except that it loads multiple rows in a slice. Also QuerySlice for now does not support loading relations due cyclic dependency.

### Upsert
This function is used to save or update existing model, if model has `primary` field and it's value is zero - this model will be inserted to the model's table. Otherwise model's row will be updated accordint it's current values. This function also supports relations except `hasMany`.
```go
err := Upsert(db, &s)
```
### Delete
This function... yea, it deletes model from database, using all it's fields except relational as identification condition. So if you loaded any model and changed it and then will call Delete nothing will hapen.

## Relations

The main goal of this package. The one and the main rule is that Model can relate only to another Model. The supported relations are:

- Has One
- Has Many
- Many To Many

To specify a `has_one` relation you need to edit field's tag:
```go 
type HasOneModel struct {
  ID int 
  Related *SomeRelatedType `ormlite:"has_one"`
}
```
Now using QueryStruct on `HasOneModel` with `LoadRelations` set to `true` will cause loading `SomeRelatedType` and setting it's pointer as a field value. To change this value just modify related model's `primary` field or set pointer to nil.

`has_many` relation is an another side of `has_one`. You can load this relation with `QueryStruct` but can not edit related model with `Upsert` since base model does not contain relation column. The requirement is that related type must have field pointing to queried type.
```go
type HasManyModel {
  ID int 
  Related []*HasOneModel `ormlite:"has_many"`
}
```
