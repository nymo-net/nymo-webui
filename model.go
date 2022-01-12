package main

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/nymo-net/nymo"
)

type baseClient = [2]interface{}
type baseServer = [2]json.RawMessage

type recvMessage struct {
	Sender   string    `json:"sender"`
	SendTime time.Time `json:"send_time"`
	Content  string    `json:"content"`
}

type newMessage struct {
	Id       int64     `json:"id"`
	Receiver string    `json:"receiver"`
	Content  string    `json:"content,omitempty"`
	SendTime time.Time `json:"send_time,omitempty"`
}

type setAlias struct {
	Address string `json:"address"`
	Name    string `json:"name"`
}

type taskDone struct {
	Task    string      `json:"task"`
	Context interface{} `json:"context"`
	Err     *string     `json:"err,omitempty"`
}

func (w *webui) broadcast(action string, msg interface{}) {
	w.wsLock.RLock()
	defer w.wsLock.RUnlock()

	for _, ch := range w.wsHandler {
		ch <- baseClient{action, msg}
	}
}

func (w *webui) taskDone(task string, ctx interface{}, err error) {
	var errStr *string
	if err != nil {
		errStr = new(string)
		*errStr = err.Error()
	}
	w.broadcast("done", taskDone{Task: task, Context: ctx, Err: errStr})
}

func (w *webui) newMessage(msg json.RawMessage) error {
	var nm newMessage
	if err := json.Unmarshal(msg, &nm); err != nil {
		return err
	}

	address := nymo.NewAddress(nm.Receiver)
	if address == nil {
		return errors.New("invalid receiver address")
	}

	id, err := w.db.lookupUserId(address.Bytes())
	if err != nil {
		return err
	}

	exec, err := w.db.Exec("INSERT INTO `send_msg` (`receiver`,`content`) VALUES (?,?)", id, nm.Content)
	if err != nil {
		return err
	}

	insertId, err := exec.LastInsertId()
	if err != nil {
		return err
	}

	nm.Id = insertId
	nm.SendTime = time.Now()
	go func() {
		w.broadcast("new_msg", nm)
		e := w.user.NewMessage(address, nm.Content)
		if e == nil {
			_, e = w.db.Exec("UPDATE `send_msg` SET `send_time`=? WHERE ROWID=?",
				nm.SendTime.UnixMilli(), insertId)
			if e != nil {
				log.Fatalf("[webui, db] %s", e)
			}
		} else {
			// no need to transmit the content back if error
			nm.Content = ""
			nm.SendTime = time.Time{}
		}
		w.taskDone("new_msg", nm, e)
	}()

	return nil
}

func (w *webui) setAlias(msg json.RawMessage) error {
	var nm setAlias
	if err := json.Unmarshal(msg, &nm); err != nil {
		return err
	}

	address := nymo.NewAddress(nm.Address)
	if address == nil {
		return errors.New("invalid address")
	}

	id, err := w.db.lookupUserId(address.Bytes())
	if err != nil {
		return err
	}

	_, err = w.db.Exec("UPDATE `user` SET `alias`=? WHERE `rowid`=?", nm.Name, id)
	if err != nil {
		return err
	}

	go w.broadcast("alias", nm)
	return nil
}
