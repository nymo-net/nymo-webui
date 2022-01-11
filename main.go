package main

import (
	"context"
	"crypto/tls"
	stdlog "log"
	"net/http"
	"os"
	"os/signal"
	"sync"

	"github.com/nymo-net/nymo"
	"github.com/sirupsen/logrus"
)

func main() {
	pair, err := tls.LoadX509KeyPair(config.Peer.TLSCert, config.Peer.TLSKey)
	if err != nil {
		log.Fatal(err)
	}

	web.db, err = openDatabase(config.Database)
	if err != nil {
		log.Fatal(err)
	}
	defer web.db.Close()

	key, err := web.db.getUserKey()
	if err != nil {
		log.Fatal(err)
	}
	web.user = nymo.OpenUser(web.db, key, pair, getCoreConfig())
	log.Infof("[core] opened user %s", web.user.Address())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var wg sync.WaitGroup

	for _, addr := range config.Peer.ListenServers {
		wg.Add(1)
		go func(a string) {
			defer wg.Done()
			log.Infof("[core] listening on %s", a)
			if err := web.user.RunServer(ctx, a); err != http.ErrServerClosed {
				log.Fatal(err)
			}
		}(addr)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		web.user.Run(ctx)
	}()

	errLogger := log.WriterLevel(logrus.ErrorLevel)
	defer errLogger.Close()
	srv := http.Server{
		Addr:     config.ListenAddr,
		Handler:  &web.m,
		ErrorLog: stdlog.New(errLogger, "[webui] ", 0),
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Infof("[webui] listening on http://%s", srv.Addr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	_ = srv.Close()
	wg.Wait()
}
