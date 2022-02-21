package main

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
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
	peer    sync.Map
}

var (
	web      = webui{wsHandler: make(map[*websocket.Conn]chan<- baseClient)}
	indexTpl = template.Must(template.New("index.gohtml").Funcs(template.FuncMap{
		"convertAddr": nymo.ConvertAddrToStr,
		"htmlEscape":  template.HTMLEscapeString,
	}).ParseFiles("./view/index.gohtml"))
)

func init() {
	web.registerRoutes()
}

func (w *webui) registerRoutes() {
	w.m.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
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
		case "history":
			var his *history
			his, err = w.getHistory(msg[1])
			if err == nil {
				msgChan <- baseClient{"history", his}
			}
		case "meta":
			m := metadata{
				Address: w.user.Address().String(),
				Servers: w.user.ListServers(),
			}
			w.peer.Range(func(_, value interface{}) bool {
				m.Peers = append(m.Peers, value.(string))
				return true
			})

			msgChan <- baseClient{"meta", m}
		default:
			err = errors.New("unknown op str")
		}
		if err != nil {
			log.Warnf("[webui] websocket process: %s", err)
			msgChan <- baseClient{"err", err.Error()}
		}
	}
}

type contact struct {
	RowID   uint
	Address []byte
	Alias   *string
}

type indexRender struct {
	Contacts []contact
}

func renderIndex(ctx context.Context, db *database, cr *indexRender) error {
	q, err := db.QueryContext(ctx, "SELECT `rowid`, `key`, `alias` FROM `user` WHERE `rowid`>0")
	if err != nil {
		return err
	}

	for q.Next() {
		var c contact
		if err := q.Scan(&c.RowID, &c.Address, &c.Alias); err != nil {
			return err
		}
		cr.Contacts = append(cr.Contacts, c)
	}

	return nil
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

	var render indexRender
	err := renderIndex(r.Context(), w.db, &render)
	if err != nil {
		http.Error(wr, err.Error(), http.StatusInternalServerError)
		return
	}

	err = indexTpl.Execute(wr, &render)
	if err != nil {
		log.Error(err)
	}
}
