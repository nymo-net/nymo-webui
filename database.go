package main

import (
	"database/sql"
	"errors"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/nymo-net/nymo"
	"github.com/nymo-net/nymo/pb"
)

type database struct {
	*sql.DB
}

func (db *database) AddPeer(url string, token []byte, cohort uint32, connected bool) {
	if connected {
		log.Printf("connected to peer %s, cohort %d", url, cohort)
	}
}

func (db *database) GetByToken(token []byte) (url string) {
	return ""
}

func (db *database) PeerDisconnected(url string, reason error) {
	log.Printf("peer %s disconnected, reason: %v", url, reason)
}

func (db *database) GetStoredPeers(cohort uint32, size uint) (tokens [][]byte, err error) {
	return nil, nil
}

func (db *database) StoreMessage(container *pb.MsgContainer, cohort uint32) {
	log.Printf("store message to cohort %d", cohort)
}

func (db *database) StoreDecryptedMessage(message *nymo.Message) {
	log.Printf("recieved message %+v", message)
}

func (db *database) ListMessages(known [][]byte) ([]*pb.MsgDigest, error) {
	return nil, nil
}

func (db *database) GetUserKey() ([]byte, error) {
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

func (db *database) checkIntegrity() error {
	_, err := db.GetUserKey()
	if err != nil {
		return err
	}
	return nil
}

func openDatabase(path string) (*database, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, errors.New("database does not exist")
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	ret := &database{db}
	err = ret.checkIntegrity()
	if err != nil {
		_ = ret.Close()
		return nil, err
	}
	return ret, nil
}

func createDatabase(path string) nymo.DatabaseFactory {
	return func(der []byte) (nymo.Database, error) {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return nil, errors.New("database already exists")
		}

		db, err := sql.Open("sqlite3", path)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				_ = db.Close()
				_ = os.Remove(path)
			}
		}()

		_, err = db.Exec("CREATE TABLE `user` (`key` BLOB NOT NULL PRIMARY KEY, `alias` TEXT);"+
			"INSERT INTO `user` (`key`) VALUES (?);", der)
		if err != nil {
			return nil, err
		}

		return &database{db}, nil
	}
}
