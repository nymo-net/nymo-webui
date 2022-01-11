package main

import (
	"encoding/json"
	"errors"
	"sync/atomic"
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
	Id       uint32 `json:"id"`
	Receiver string `json:"receiver"`
	Content  string `json:"content"`
}

type taskDone struct {
	Id  uint32  `json:"id"`
	Err *string `json:"err"`
}

func (w *webui) broadcast(action string, msg interface{}) {
	w.wsLock.RLock()
	defer w.wsLock.RUnlock()

	for _, ch := range w.wsHandler {
		ch <- baseClient{action, msg}
	}
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

	go func() {
		nm.Id = atomic.AddUint32(&w.counter, 1)
		w.broadcast("new_msg", nm)

		err := w.user.NewMessage(address, nm.Content)
		var errStr *string
		if err != nil {
			errStr = new(string)
			*errStr = err.Error()
		}
		w.broadcast("done", taskDone{Id: nm.Id, Err: errStr})
	}()

	return nil
}
