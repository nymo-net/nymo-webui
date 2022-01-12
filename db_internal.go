package main

func (db *database) lookupUserId(key []byte) (uint, error) {
	_, err := db.Exec("INSERT OR IGNORE INTO `user` (`key`) VALUES (?)", key)
	if err != nil {
		return 0, err
	}
	row := db.QueryRow("SELECT `rowid` FROM `user` WHERE `key`=?", key)
	if row.Err() != nil {
		return 0, row.Err()
	}
	var id uint
	err = row.Scan(&id)
	return id, err
}
