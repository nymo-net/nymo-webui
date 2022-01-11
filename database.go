package main

import (
	"database/sql"
	"errors"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nymo-net/nymo"
	"github.com/nymo-net/nymo/pb"
)

type database struct {
	*sql.DB
	storeLock sync.Mutex
}

func (db *database) MessageStat(cohort uint32) (uint, uint) {
	row := db.QueryRow(
		"SELECT COUNT(CASE WHEN `cohort`=? THEN 1 END), COUNT(*) FROM `message` WHERE `msg` IS NOT NULL", cohort)
	if row.Err() != nil {
		log.Panic(row.Err())
	}

	var in, total uint
	err := row.Scan(&in, &total)
	if err != nil {
		log.Panic(err)
	}

	return in, total - in
}

func (db *database) ClientHandle(id []byte) nymo.PeerHandle {
	rowId, err := getPeerRowId(db.DB, id)
	if err != nil {
		log.Panic(err)
	}
	log.WithField("id", id).Debug("[webui] client connected")
	return &peerHandle{db: db.DB, row: rowId}
}

func (db *database) AddPeer(url string, digest *pb.Digest) {
	_, err := db.Exec("REPLACE INTO `peer_link` (`url_hash`,`url`,`cohort`) VALUES (?,?,?)",
		digest.Hash, url, digest.Cohort)
	if err != nil {
		log.Panic(err)
	}
}

func (db *database) EnumeratePeers() nymo.PeerEnumerate {
	query, err := db.Query("SELECT `url_hash`, `url`, `cohort` FROM `peer_link` ORDER BY `penalize`")
	if err != nil {
		log.Panic(err)
	}
	return &peerEnum{
		db:   db.DB,
		rows: query,
	}
}

func (db *database) GetUrlByHash(urlHash []byte) (url string) {
	row := db.QueryRow("SELECT `url` FROM `peer_link` WHERE `url_hash`=?", urlHash)
	if row.Err() != nil {
		log.Panic(row.Err())
	}

	err := row.Scan(&url)
	if err != nil {
		log.Panic(err)
	}
	return
}

func (db *database) GetMessage(hash []byte) (msg []byte, pow uint64) {
	row := db.QueryRow("SELECT `msg`, `pow` FROM `message` WHERE `hash`=?", hash)
	if row.Err() != nil {
		log.Panic(row.Err())
	}

	err := row.Scan(&msg, &pow)
	if err != nil {
		log.Panic(err)
	}
	return
}

func (db *database) StoreMessage(hash []byte, c *pb.MsgContainer, f func() (cohort uint32, err error)) error {
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	row := db.QueryRow("SELECT COUNT(*) FROM `message` WHERE `hash`=? AND `msg` IS NOT NULL", hash)
	if row.Err() != nil {
		return row.Err()
	}

	var cnt int
	if err := row.Scan(&cnt); err != nil {
		return err
	}

	if cnt != 0 {
		return nil
	}

	cohort, err := f()
	if err != nil {
		return err
	}

	_, err = db.Exec("REPLACE INTO `message` (`hash`,`msg`,`pow`,`cohort`) VALUES (?,?,?,?)",
		hash, c.Msg, c.Pow, cohort)
	return err
}

func (db *database) StoreDecryptedMessage(message *nymo.Message) {
	_, err := db.Exec("INSERT INTO `dec_msg` VALUES (?,?,?)",
		message.Sender.Bytes(), message.Content, message.SendTime.UnixMilli())
	if err != nil {
		log.Panic(err)
	}
	go web.broadcast("recv_msg", recvMessage{
		Sender:   message.Sender.String(),
		SendTime: message.SendTime,
		Content:  message.Content,
	})
}

func (db *database) getUserKey() ([]byte, error) {
	query, err := db.Query("SELECT `key` FROM `user` WHERE ROWID=1")
	if err != nil {
		return nil, err
	}
	defer query.Close()

	if !query.Next() {
		return nil, errors.New("user key not found")
	}

	var ret []byte
	return ret, query.Scan(&ret)
}

const dbOptions = "?_foreign_keys=on&_journal_mode=wal"

func openDatabase(path string) (*database, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("database does not exist")
	}

	db, err := sql.Open("sqlite3", path+dbOptions)
	if err != nil {
		return nil, err
	}
	return &database{DB: db}, nil
}

func createDatabase(path string, der []byte) error {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.New("database already exists")
	}

	db, err := sql.Open("sqlite3", path+dbOptions)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
		if err != nil {
			_ = os.Remove(path)
		}
	}()

	_, err = db.Exec(schema)
	if err == nil {
		_, err = db.Exec("INSERT INTO `user` (`key`) VALUES (?);", der)
	}

	return err
}

// language=sql
const schema = `CREATE TABLE "user"
(
	"key" BLOB PRIMARY KEY,
	"alias" TEXT
);

CREATE TABLE "peer"
(
	"rowid" INTEGER PRIMARY KEY,
	"id" BLOB NOT NULL
);

CREATE UNIQUE INDEX "peer_id_uindex" on "peer" ("id");

CREATE TABLE "dec_msg"
(
	"sender" BLOB NOT NULL,
	"content" TEXT NOT NULL,
	"send_time" INTEGER NOT NULL
);

CREATE TABLE "message"
(
	"rowid" INTEGER PRIMARY KEY,
	"hash" BLOB NOT NULL,
	"msg" BLOB,
	"pow" INTEGER,
	"cohort" INTEGER NOT NULL,
	"deleted" BOOLEAN DEFAULT FALSE NOT NULL
);

CREATE UNIQUE INDEX "message_hash_uindex" on "message" ("hash");

CREATE TABLE "peer_link"
(
	"url_hash" BLOB PRIMARY KEY,
	"url" TEXT NOT NULL,
	"cohort" INTEGER NOT NULL,
	"penalize" INTEGER DEFAULT 0 NOT NULL
) WITHOUT ROWID;

CREATE INDEX "peer_link_cohort_index" ON "peer_link" ("cohort");

CREATE TABLE "known_msg"
(
	"peer_id" INTEGER NOT NULL
		REFERENCES "peer" ON UPDATE CASCADE ON DELETE CASCADE,
	"msg" INTEGER NOT NULL
		REFERENCES "message" ON UPDATE CASCADE ON DELETE CASCADE,
	PRIMARY KEY ("peer_id", "msg")
) WITHOUT ROWID;

CREATE TABLE "known_peer"
(
	"peer_id" INTEGER NOT NULL
		REFERENCES "peer" ON UPDATE CASCADE ON DELETE CASCADE,
	"url_hash" BLOB NOT NULL,
	PRIMARY KEY ("peer_id", "url_hash")
) WITHOUT ROWID;`
