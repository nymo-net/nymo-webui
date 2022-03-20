package main

import (
	"database/sql"
	"encoding/hex"
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

func (db *database) IgnoreMessage(digest *pb.Digest) {
	_, err := db.Exec("INSERT OR IGNORE INTO `message` (`hash`,`cohort`,`deleted`) VALUES(?,?,TRUE)",
		digest.Hash, digest.Cohort)
	if err != nil {
		log.Panic(err)
	}
}

func (db *database) ClientHandle(id [8]byte) nymo.PeerHandle {
	rowId, err := getPeerRowId(db.DB, id[:])
	if err != nil {
		log.Panic(err)
	}
	encoded := hex.EncodeToString(id[:])
	log.WithField("id", encoded).Debug("[core] client connected")
	web.peer.Store(rowId, encoded)
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

func (db *database) GetUrlByHash(urlHash [8]byte) (url string) {
	row := db.QueryRow("SELECT `url` FROM `peer_link` WHERE `url_hash`=?", urlHash[:])
	if row.Err() != nil {
		log.Panic(row.Err())
	}

	err := row.Scan(&url)
	if err != nil {
		log.Panic(err)
	}
	return
}

func (db *database) GetMessage(hash [32]byte) (msg []byte, pow uint64) {
	row := db.QueryRow("SELECT `msg`, `pow` FROM `message` WHERE `hash`=?", hash[:])
	if row.Err() != nil {
		log.Panic(row.Err())
	}

	err := row.Scan(&msg, &pow)
	if err != nil {
		log.Panic(err)
	}
	return
}

func (db *database) StoreMessage(hash [32]byte, c *pb.MsgContainer, f func() (cohort uint32, err error)) error {
	db.storeLock.Lock()
	defer db.storeLock.Unlock()

	row := db.QueryRow("SELECT COUNT(*) FROM `message` WHERE `hash`=? AND `msg` IS NOT NULL", hash[:])
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

	_, err = db.Exec("INSERT INTO `message` (`hash`,`cohort`,`msg`,`pow`) VALUES (?,?,@msg,@pow) ON CONFLICT DO UPDATE SET `msg`=@msg,`pow`=@pow",
		hash[:], cohort, sql.Named("msg", c.Msg), sql.Named("pow", c.Pow))
	return err
}

func (db *database) StoreDecryptedMessage(message *nymo.Message) {
	target, err := db.lookupUserId(message.Sender.Bytes())
	if err != nil {
		log.Panic(err)
	}
	_, err = db.Exec("INSERT INTO `dec_msg` VALUES (?,FALSE,?,?)",
		target, message.Content, message.SendTime.UnixMilli())
	if err != nil {
		log.Panic(err)
	}
	go web.recvMessage(target, string(message.Content), message.SendTime)
}

func (db *database) getUserKey() ([]byte, error) {
	query, err := db.Query("SELECT `key` FROM `user` WHERE `rowid`=0")
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
		_, err = db.Exec("INSERT INTO `user` (`rowid`, `key`) VALUES (0, ?);", der)
	}

	return err
}

// language=sql
const schema = `CREATE TABLE "user"
(
	"rowid" INTEGER PRIMARY KEY,
	"key" BLOB UNIQUE NOT NULL,
	"alias" TEXT
);

CREATE TABLE "peer"
(
	"rowid" INTEGER PRIMARY KEY,
	"id" BLOB UNIQUE NOT NULL
);

CREATE TABLE "dec_msg"
(
	"target" INTEGER NOT NULL
		REFERENCES "user" ON UPDATE CASCADE ON DELETE CASCADE,
	"self" BOOLEAN NOT NULL,
	"content" TEXT NOT NULL,
	"send_time" INTEGER
);

CREATE TABLE "message"
(
	"rowid" INTEGER PRIMARY KEY,
	"hash" BLOB NOT NULL,
	"cohort" INTEGER NOT NULL,
	"msg" BLOB,
	"pow" INTEGER,
	"deleted" BOOLEAN DEFAULT FALSE NOT NULL,
	UNIQUE ("hash", "cohort")
);

CREATE TABLE "peer_link"
(
	"url_hash" BLOB PRIMARY KEY,
	"url" TEXT NOT NULL,
	"cohort" INTEGER NOT NULL,
	"penalize" INTEGER DEFAULT 0 NOT NULL
) WITHOUT ROWID;

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
