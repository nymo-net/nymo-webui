package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/nymo-net/nymo"
)

type webui struct {
	m http.ServeMux
	u websocket.Upgrader

	user *nymo.User
	db   *database

	wsLock    sync.RWMutex
	wsHandler map[*websocket.Conn]chan<- baseClient

	counter uint32
}

var web = webui{
	wsHandler: make(map[*websocket.Conn]chan<- baseClient),
}

func init() {
	web.registerRoutes()
}

func (w *webui) registerRoutes() {
	w.m.HandleFunc("/", w.serveIndex)
}

func (w *webui) websocketHandle(conn *websocket.Conn, msgChan chan baseClient) {
	w.wsLock.Lock()
	w.wsHandler[conn] = msgChan
	w.wsLock.Unlock()

	defer func() {
		w.wsLock.Lock()
		delete(w.wsHandler, conn)
		w.wsLock.Unlock()
		close(msgChan)
	}()

	go func() {
		defer conn.Close()

		for msg := range msgChan {
			if err := conn.WriteJSON(msg); err != nil {
				if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
					log.Errorf("[webui] websocket: %s", err)
				} else {
					log.Debugf("[webui] websocket: %s", err)
				}
				return
			}
		}
	}()

	for {
		var msg baseServer
		if err := conn.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
				log.Errorf("[webui] websocket: %s", err)
			} else {
				log.Debugf("[webui] websocket: %s", err)
			}
			return
		}

		var action string
		if err := json.Unmarshal(msg[0], &action); err != nil {
			log.Errorf("[webui] websocket: %s", err)
			return
		}

		var err error
		switch action {
		case "new_msg":
			err = w.newMessage(msg[1])
		case "alias":
			err = w.setAlias(msg[1])
		default:
			err = errors.New("unknown op str")
		}
		if err != nil {
			log.Warnf("[webui] websocket process: %s", err)
			msgChan <- baseClient{"err", err.Error()}
		}
	}
}

func (w *webui) serveIndex(wr http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(wr, r)
		return
	}

	if websocket.IsWebSocketUpgrade(r) {
		conn, err := w.u.Upgrade(wr, r, nil)
		if err != nil {
			log.Warn(err)
		} else {
			go w.websocketHandle(conn, make(chan baseClient, 10))
		}
		return
	}
}
