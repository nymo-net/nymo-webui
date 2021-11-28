package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/nymo-net/nymo"
)

const (
	certPath = "./nymo.crt"
	keyPath  = "./nymo.key"
	dbPath   = "./nymo.db"
)

func help() {
	fmt.Fprintln(os.Stderr, "Usage: nymo [listen | connect] address")
	os.Exit(1)
}

func init() {
	if len(os.Args) < 3 {
		help()
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		_, err = nymo.GenerateUser(createDatabase(dbPath), nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			log.Fatal(err)
		}
		cert, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
			Subject: pkix.Name{
				Country:            nil,
				Organization:       nil,
				OrganizationalUnit: nil,
				Locality:           nil,
				Province:           nil,
				StreetAddress:      nil,
				PostalCode:         nil,
				SerialNumber:       "",
				CommonName:         "",
				Names:              nil,
				ExtraNames:         nil,
			},
			SerialNumber: big.NewInt(0),
		}, &x509.Certificate{}, &key.PublicKey, key)
		if err != nil {
			log.Fatal(err)
		}
		certFile, err := os.Create(certPath)
		if err != nil {
			log.Fatal(err)
		}
		defer certFile.Close()

		err = pem.Encode(certFile, &pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		})
		if err != nil {
			log.Fatal(err)
		}

		keyFile, err := os.Create(keyPath)
		if err != nil {
			log.Fatal(err)
		}
		defer keyFile.Close()

		err = pem.Encode(keyFile, &pem.Block{
			Type:    "RSA PRIVATE KEY",
			Bytes:   x509.MarshalPKCS1PrivateKey(key),
		})
		if err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	db, err := openDatabase(dbPath)
	if err != nil {
		log.Fatal(err)
	}

	user, err := nymo.OpenUser(db, nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Your Address:", user.Address())

	switch os.Args[1] {
	case "listen":
		log.Fatal(user.RunServer(os.Args[2], certPath, keyPath))
	case "connect":
	default:
		help()
	}

	peer, err := user.DialPeer(os.Args[2])
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("Target Address: ")
		if !scanner.Scan() {
			break
		}
		addr := nymo.NewAddress(scanner.Text())
		if addr == nil {
			fmt.Println("Invalid Address!")
			continue
		}

		fmt.Print("Message: ")
		if !scanner.Scan() {
			break
		}
		msg := scanner.Text()

		message, err := user.NewMessage(addr, msg)
		if err != nil {
			log.Fatal(err)
		}

		err = peer.SendMessage(message)
		if err != nil {
			log.Fatal(err)
		}
	}
}
