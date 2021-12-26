package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"

	"github.com/nymo-net/nymo"
)

const (
	certPath = "./nymo.crt"
	keyPath  = "./nymo.key"
	dbPath   = "./nymo.db"
)

func help() {
	fmt.Fprintln(os.Stderr, "Usage: nymo [address]")
	os.Exit(1)
}

func init() {
	if len(os.Args) < 2 {
		help()
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		key, err := nymo.GenerateUser()
		if err != nil {
			log.Panic(err)
		}
		err = createDatabase(dbPath, key)
		if err != nil {
			log.Panic(err)
		}
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			log.Panic(err)
		}
		cert, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
			SerialNumber: big.NewInt(0),
		}, &x509.Certificate{}, &key.PublicKey, key)
		if err != nil {
			log.Panic(err)
		}
		certFile, err := os.Create(certPath)
		if err != nil {
			log.Panic(err)
		}
		defer certFile.Close()

		err = pem.Encode(certFile, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		})
		if err != nil {
			log.Panic(err)
		}

		keyFile, err := os.Create(keyPath)
		if err != nil {
			log.Panic(err)
		}
		defer keyFile.Close()

		err = pem.Encode(keyFile, &pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
		if err != nil {
			log.Panic(err)
		}
	}
}

func main() {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		log.Panic(err)
	}

	db, err := openDatabase(dbPath)
	if err != nil {
		log.Panic(err)
	}
	defer db.Close()

	key, err := db.getUserKey()
	if err != nil {
		log.Panic(err)
	}
	user := nymo.OpenUser(db, key, pair, nil)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		if err := user.RunServer(ctx, os.Args[1]); err != http.ErrServerClosed {
			log.Panic(err)
		}
	}()

	fmt.Println("Address:", user.Address())
	user.Run(ctx)
}
