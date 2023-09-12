package datastore

import (
	"errors"
	"github.com/mattn/go-sqlite3"
)

func asInternalError(err error, errcode sqlite3.ErrNoExtended) bool {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) {
		if sqliteErr.ExtendedCode == errcode {
			return true
		}
	}
	return false
}

func IsUniqueConstraintError(err error) bool {
	return asInternalError(err, sqlite3.ErrConstraintUnique)
}
