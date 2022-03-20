package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nymo-net/nymo"
)

type baseClient = [2]interface{}
type baseServer = [2]json.RawMessage

type newMessage struct {
	Target  interface{} `json:"target"`
	Content string      `json:"content"`
}

type msgRender struct {
	Self      bool
	Content   string
	SendTime  *time.Time
	PrepareId int64
}

func (w *webui) recvMessage(target uint, content string, sendTime time.Time) {
	var buf bytes.Buffer
	err := indexTpl.ExecuteTemplate(&buf, "message", msgRender{Content: content, SendTime: &sendTime})
	if err != nil {
		log.Fatal(err)
	}
	w.broadcast("new_msg", newMessage{
		Target:  target,
		Content: buf.String(),
	})
}

type setAlias struct {
	Id   uint    `json:"id"`
	Name *string `json:"name,omitempty"`
}

type metadata struct {
	Version string   `json:"version"`
	Address string   `json:"address"`
	Peers   []string `json:"peers"`
	Servers []string `json:"servers"`
}

type msgSent struct {
	Target  uint    `json:"target"`
	Id      int64   `json:"id"`
	Content string  `json:"content,omitempty"`
	Err     *string `json:"err,omitempty"`
}

func (w *webui) broadcast(action string, msg interface{}) {
	w.wsLock.RLock()
	defer w.wsLock.RUnlock()

	for _, ch := range w.wsHandler {
		ch <- baseClient{action, msg}
	}
}

func (w *webui) msgSent(target uint, id int64, content string, err error) {
	var errStr *string
	if err != nil {
		errStr = new(string)
		*errStr = err.Error()
	}
	w.broadcast("msg_sent", msgSent{
		Target:  target,
		Id:      id,
		Content: content,
		Err:     errStr,
	})
}

func (w *webui) newUser(row uint, id []byte) {
	var buf bytes.Buffer
	err := indexTpl.ExecuteTemplate(&buf, "contact", contact{
		RowID:   row,
		Address: id,
	})
	if err != nil {
		log.Fatal(err)
	}
	w.broadcast("new_user", buf.String())
}

func (w *webui) newMessage(msg json.RawMessage) error {
	var nm newMessage
	if err := json.Unmarshal(msg, &nm); err != nil {
		return err
	}

	var address *nymo.Address
	switch addr := nm.Target.(type) {
	case float64:
		target := uint(addr)
		if target <= 0 {
			return errors.New("invalid receiver id")
		}

		row := w.db.QueryRow("SELECT `key` FROM `user` WHERE `rowid`=?", target)
		if row.Err() != nil {
			return row.Err()
		}
		var receiver []byte
		if err := row.Scan(&receiver); err != nil {
			return err
		}

		address = nymo.NewAddressFromBytes(receiver)
		if address == nil {
			log.Fatal("[webui, db] invalid receiver address")
		}
		nm.Target = target
	case string:
		address = nymo.NewAddress(addr)
		if address == nil {
			return errors.New("invalid receiver address")
		}
		var err error
		nm.Target, err = w.db.lookupUserId(address.Bytes())
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown receiver type %T", addr)
	}

	exec, err := w.db.Exec("INSERT INTO `dec_msg` (`target`,`self`,`content`) VALUES (?,TRUE,?)", nm.Target, nm.Content)
	if err != nil {
		return err
	}

	insertId, err := exec.LastInsertId()
	if err != nil {
		return err
	}

	go func() {
		var buf bytes.Buffer
		e := indexTpl.ExecuteTemplate(&buf, "message", msgRender{
			Self:      true,
			Content:   nm.Content,
			PrepareId: insertId,
		})
		if e != nil {
			log.Fatalf("[webui, template] %s", e)
		}
		oriContent := nm.Content
		nm.Content = buf.String()
		buf.Reset()
		w.broadcast("new_msg", nm)

		sendTime := time.Now()
		e = w.user.NewMessage(address, []byte(oriContent))
		if e == nil {
			_, e = w.db.Exec("UPDATE `dec_msg` SET `send_time`=? WHERE ROWID=?", sendTime.UnixMilli(), insertId)
			if e != nil {
				log.Fatalf("[webui, db] %s", e)
			}
			e = indexTpl.ExecuteTemplate(&buf, "message", msgRender{
				Self:     true,
				Content:  oriContent,
				SendTime: &sendTime,
			})
			if e != nil {
				log.Fatalf("[webui, template] %s", e)
			}
		} else {
			_, e = w.db.Exec("DELETE FROM `dec_msg` WHERE ROWID=?", insertId)
			if e != nil {
				log.Fatalf("[webui, db] %s", e)
			}
		}
		w.msgSent(nm.Target.(uint), insertId, buf.String(), e)
	}()

	return nil
}

func (w *webui) setAlias(msg json.RawMessage) error {
	var nm setAlias
	if err := json.Unmarshal(msg, &nm); err != nil {
		return err
	}

	_, err := w.db.Exec("UPDATE `user` SET `alias`=? WHERE `rowid`=?", nm.Name, nm.Id)
	if err != nil {
		return err
	}

	go w.broadcast("alias", nm)
	return nil
}

type history struct {
	Id      uint   `json:"id"`
	Content string `json:"content"`
}

func (w *webui) getHistory(msg json.RawMessage) (*history, error) {
	var id uint
	if err := json.Unmarshal(msg, &id); err != nil {
		return nil, err
	}

	query, err := w.db.Query(
		"SELECT ROWID, `self`, `content`, `send_time` FROM `dec_msg` WHERE `target`=? ORDER BY ROWID DESC", id)
	if err != nil {
		return nil, err
	}

	var msgs []msgRender
	for query.Next() {
		var r msgRender
		var t *int64
		err = query.Scan(&r.PrepareId, &r.Self, &r.Content, &t)
		if err != nil {
			return nil, err
		}
		if t != nil {
			r.SendTime = new(time.Time)
			*r.SendTime = time.UnixMilli(*t)
		}
		msgs = append(msgs, r)
	}

	var buf bytes.Buffer
	err = indexTpl.ExecuteTemplate(&buf, "messages", msgs)
	if err != nil {
		log.Fatal(err)
	}
	return &history{
		Id:      id,
		Content: buf.String(),
	}, nil
}
