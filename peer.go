package main

import (
	"bytes"
	"database/sql"

	"github.com/nymo-net/nymo"
	"github.com/nymo-net/nymo/pb"
)

func getPeerRowId(db *sql.DB, id []byte) (uint, error) {
	row, _, err := insertOrIgnore(db,
		"INSERT OR IGNORE INTO `peer` (`id`) VALUES (?)",
		"SELECT `rowid` FROM `peer` WHERE `id`=?", id)
	return row, err
}

func digestToTable(tx *sql.Tx, digests []*pb.Digest) {
	_, err := tx.Exec("CREATE TEMP TABLE `digest` (`hash` BLOB PRIMARY KEY, `cohort` INTEGER) WITHOUT ROWID")
	if err != nil {
		log.Panic(err)
	}

	var sqlStr bytes.Buffer
	sqlStr.WriteString("INSERT INTO `digest` VALUES")
	var vals []interface{}

	for _, d := range digests {
		sqlStr.WriteString(`(?,?),`)
		vals = append(vals, d.Hash, d.Cohort)
	}
	sqlStr.Truncate(sqlStr.Len() - 1)

	_, err = tx.Exec(sqlStr.String(), vals...)
	if err != nil {
		log.Panic(err)
	}
}

func extractDigest(query *sql.Rows) (ret []*pb.Digest) {
	defer query.Close()
	for query.Next() {
		dig := new(pb.Digest)
		err := query.Scan(&dig.Hash, &dig.Cohort)
		if err != nil {
			log.Panic(err)
		}
		ret = append(ret, dig)
	}
	if query.Err() != nil {
		log.Panic(query.Err())
	}
	return
}

func extractRowIdAndDigest(query *sql.Rows) (r []uint, d []*pb.Digest) {
	defer query.Close()
	var id uint
	for query.Next() {
		dig := new(pb.Digest)
		err := query.Scan(&id, &dig.Hash, &dig.Cohort)
		if err != nil {
			log.Panic(err)
		}
		r = append(r, id)
		d = append(d, dig)
	}
	if query.Err() != nil {
		log.Panic(query.Err())
	}
	return
}

type peerHandle struct {
	db   *sql.DB
	row  uint
	last []uint
}

func (p *peerHandle) AddKnownMessages(digests []*pb.Digest) []*pb.Digest {
	if len(digests) <= 0 {
		return nil
	}

	tx, err := p.db.Begin()
	defer tx.Rollback()

	// 1. create temp table
	digestToTable(tx, digests)

	// 2. insert it into message
	_, err = tx.Exec("INSERT OR IGNORE INTO `message` (`hash`, `cohort`) SELECT * FROM `digest`")
	if err != nil {
		log.Panic(err)
	}

	// 3. intermediate rowid table
	_, err = tx.Exec("CREATE TEMP TABLE `interm` AS SELECT `rowid` FROM `message` WHERE `hash` IN (SELECT `hash` FROM `digest`)")
	if err != nil {
		log.Panic(err)
	}

	// 4. insert it into known_msg
	_, err = tx.Exec("INSERT OR IGNORE INTO `known_msg` SELECT ?, `rowid` FROM `interm`", p.row)
	if err != nil {
		log.Panic(err)
	}

	// 5. find what we don't know
	query, err := tx.Query("SELECT `hash`, `cohort` FROM `message` WHERE (NOT `deleted`) AND (`msg` IS NULL) AND `rowid` IN (SELECT `rowid` FROM `interm`)")
	if err != nil {
		log.Panic(err)
	}
	need := extractDigest(query)

	// 6. drop temp tables
	_, err = tx.Exec("DROP TABLE `digest`; DROP TABLE `interm`")
	if err != nil {
		log.Panic(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Panic(err)
	}
	return need
}

func (p *peerHandle) ListMessages(size uint) []*pb.Digest {
	query, err := p.db.Query("SELECT `rowid`, `hash`, `cohort` FROM `message` WHERE (`msg` IS NOT NULL) AND `rowid` NOT IN (SELECT `msg` FROM `known_msg` WHERE `peer_id`=?) LIMIT ?", p.row, size)
	if err != nil {
		log.Panic(err)
	}
	id, digests := extractRowIdAndDigest(query)
	p.last = id
	return digests
}

func (p *peerHandle) AckMessages() {
	if len(p.last) <= 0 {
		return
	}
	var sqlStr bytes.Buffer
	sqlStr.WriteString("INSERT OR IGNORE INTO `known_msg` SELECT ?, `column1` FROM (VALUES ")
	vals := []interface{}{p.row}

	for _, d := range p.last {
		sqlStr.WriteString("(?),")
		vals = append(vals, d)
	}
	bs := sqlStr.Bytes()
	bs[len(bs)-1] = ')'

	_, err := p.db.Exec(string(bs), vals...)
	if err != nil {
		log.Panic(err)
	}
	p.last = nil
}

func (p *peerHandle) AddKnownPeers(digests []*pb.Digest) []*pb.Digest {
	if len(digests) <= 0 {
		return nil
	}

	tx, err := p.db.Begin()
	defer tx.Rollback()

	// 1. create temp table
	digestToTable(tx, digests)

	// 2. insert it into known_peer
	_, err = tx.Exec("INSERT OR IGNORE INTO `known_peer` SELECT ?, `hash` FROM `digest`", p.row)
	if err != nil {
		log.Panic(err)
	}

	// 3. find what we don't know
	query, err := tx.Query("SELECT * FROM `digest` WHERE `hash` NOT IN (SELECT `url_hash` FROM `peer_link`)")
	if err != nil {
		log.Panic(err)
	}
	ret := extractDigest(query)

	// 4. drop temp table
	_, err = tx.Exec("DROP TABLE `digest`")
	if err != nil {
		log.Panic(err)
	}

	err = tx.Commit()
	if err != nil {
		log.Panic(err)
	}
	return ret
}

func (p *peerHandle) ListPeers(size uint) []*pb.Digest {
	query, err := p.db.Query("SELECT `url_hash`, `cohort` FROM `peer_link` WHERE `url_hash` NOT IN (SELECT `url_hash` FROM `known_peer` WHERE `peer_id`=?) ORDER BY `penalize` LIMIT ?", p.row, size)
	if err != nil {
		log.Panic(err)
	}
	return extractDigest(query)
}

func (p *peerHandle) Disconnect(err error) {
	// TODO penalize
	web.peer.Delete(p.row)
	log.WithError(err).Debug("[core] peer disconnected")
}

type peerEnum struct {
	hash   []byte
	url    string
	cohort uint32
	db     *sql.DB
	rows   *sql.Rows
}

func (p *peerEnum) Url() string {
	return p.url
}

func (p *peerEnum) Cohort() uint32 {
	return p.cohort
}

func (p *peerEnum) Next(err error) bool {
	if err != nil {
		_, err := p.db.Exec("UPDATE `peer_link` SET `penalize`=`penalize`+1 WHERE `url_hash`=?", p.hash)
		if err != nil {
			log.Panic(err)
		}
	}
	if p.rows.Next() {
		err := p.rows.Scan(&p.hash, &p.url, &p.cohort)
		if err != nil {
			log.Panic(err)
		}
		return true
	}
	return false
}

func (p *peerEnum) Connect(id []byte, cohort uint32) nymo.PeerHandle {
	_, err := p.db.Exec("UPDATE `peer_link` SET `cohort`=? WHERE `url_hash`=?", cohort, p.hash)
	if err != nil {
		log.Panic(err)
	}
	rowId, err := getPeerRowId(p.db, id)
	if err != nil {
		log.Panic(err)
	}
	log.WithField("id", id).Debug("[core] peer connected")
	web.peer.Store(rowId, p.url)
	return &peerHandle{
		db:  p.db,
		row: rowId,
	}
}

func (p *peerEnum) Close() {
	_ = p.rows.Close()
}
