# ormlite
Lightweight package implementing some ORM-like features and helpers for sqlite databases.

[![Build Status](https://app.travis-ci.com/pupizoid/ormlite.svg?branch=master)](https://app.travis-ci.com/pupizoid/ormlite)
[![GoDoc](https://godoc.org/github.com/pupizoid/ormlite?status.svg)](https://godoc.org/github.com/pupizoid/ormlite)
[![Go Report Card](https://goreportcard.com/badge/github.com/pupizoid/ormlite)](https://goreportcard.com/report/github.com/pupizoid/ormlite)
[![Awesome](https://cdn.rawgit.com/sindresorhus/awesome/d7305f38d29fed78fa85652e3a63e154dd8e8829/media/badge.svg)](https://github.com/avelino/awesome-go)

## Model
```go
type Model interface {
    Table() string
}
```
This package operates models which are described by `Model` interface. We call any entry a model if it's a struct and has a table where data is stored.

## CRUD
This package provides a bunch of functions to allow you create, read, update and delete data.
  
### QueryStruct
Loads data from table and scans it into provided struct. If query was too broad to load more than one rows, the latest of them will be scanned. Also this function supports loading relations which will be described below.

```go
type SimpleStruct struct {
  IntField int64 `ormlite:"col=rowid,primary"`
  Text string
  UnusedField bool `ormlite:"-"
}

var s SimpleStruct
err := QueryStruct(db, "", nil, &s)
```

Let's describe some tags used in example struct:
- `col` - let you specify custom column name to be scanned to the field
- `primary` - indicates model primary key, it's basically used when saving model
- `-` - hide field for package so it won't be affected at any kind

### QuerySlice
This is very similar to QueryStruct except that it loads multiple rows in a slice.

### Upsert
This function is used to save or update existing model, if model has `primary` field and it's value is zero - this model will be inserted to the model's table. Otherwise model's row will be updated according it's current values (except `has-one` relation). This function also supports updating related models except creating or editing `many-to-many` related models.
```go
err := Upsert(db, &s)
```

### Insert 
Function used for inserting Models. Despite of `Upsert` it returns an error in case of constraint errors. 

### Delete
This function... yea, it deletes model from database using it's primary key value. If model does not have primary key or it has zero value an error will ne returned.
Since sometimes it's useful to know that delete operation is really took place in database, function will check number of affected rows and return a special `ErrNoRowsAffected`
if it's not positive.

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

For example:

```go
opts := ormlite.WithWhere(ormlite.DefaultOptions(), ormlite.Where{"id": 1})
```

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
   Related       []ormlite.Model `ormlite:"many_to_many,table=mapping_table,field=model_id"`
   RelatedActive []ormlite.Model `ormlite:"many_to_many,table=mapping_table(active=1),field=model_id"`
}
```

`many_to_many` indicates that field represents many to many relation.

`table(additional condition)` should contain mapping table name to retrieve relation information. If it's necessary to map entities with additional conditions you can specify sql describing them in brackets. For now only one additional field is supported.

`field` should specify column in mapping table that has foreign key of original model

Also there is a requirement to related model primary key field to contain `ref` setting that specifies column name of it's foreign key in mapping table.

### Examples

See tests.
