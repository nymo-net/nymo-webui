package main

import "database/sql"

func insertOrIgnore(db *sql.DB, ins, sel string, arg interface{}) (uint, bool, error) {
	exec, err := db.Exec(ins, arg)
	if err != nil {
		return 0, false, err
	}
	affected, err := exec.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	if affected != 0 {
		insertId, err := exec.LastInsertId()
		return uint(insertId), true, err
	}

	row := db.QueryRow(sel, arg)
	if row.Err() != nil {
		return 0, false, row.Err()
	}
	var id uint
	err = row.Scan(&id)
	return id, false, err
}

func (db *database) lookupUserId(key []byte) (uint, error) {
	id, inserted, err := insertOrIgnore(db.DB,
		"INSERT OR IGNORE INTO `user` (`key`) VALUES (?)",
		"SELECT `rowid` FROM `user` WHERE `key`=?", key)
	if inserted {
		web.newUser(id, key)
	}
	return id, err
}
