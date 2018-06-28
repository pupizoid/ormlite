# ormlite
Simple package that contains some ORM like functionality for `database/sql` espeshially for sqlite3

[![Build Status](https://travis-ci.org/pupizoid/ormlite.svg?branch=master)](https://travis-ci.org/pupizoid/ormlite)
[![codecov](https://codecov.io/gh/pupizoid/ormlite/branch/master/graph/badge.svg)](https://codecov.io/gh/pupizoid/ormlite)
[![GoDoc](https://godoc.org/github.com/pupizoid/ormlite?status.svg)](https://godoc.org/github.com/pupizoid/ormlite)
[![Go Report Card](https://goreportcard.com/badge/github.com/pupizoid/ormlite)](https://goreportcard.com/report/github.com/pupizoid/ormlite)

# tag options

- `primary` - Use this options to indicate the field representing model's primary key. It will be used loading relations.
- `col` - Specifies custom column name for recent field, if not used lowercase of field name is used.
- `ref` - Specifies name of the column in additional relation table, usually used for many-to-many relations and in the same field as `primary`
- `table` - Specifies table name where relation's data is stored. Used for `many_to_many` relations.
- `field` - Specifies model's column name for relation table.
- `one_to_one` - Indicates that field represents one to one relation. Field type should be pointer to another type implementing `Model` interface. Only one type of relations can be set for single field.
- `many_to_many` - Indicates that field represents many to many relation. Field type should be slice of `Model`. Must be combinated with `table` and `field` options. 