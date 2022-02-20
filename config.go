package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	stdlog "log"
	"math/big"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/nymo-net/nymo"
	"github.com/sirupsen/logrus"
)

var (
	config tomlConfig

	log = logrus.New()
)

type duration time.Duration

func (d *duration) UnmarshalText(text []byte) error {
	dur, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = duration(dur)
	return nil
}

type tomlConfig struct {
	ListenAddr string       `toml:"listen_addr"`
	Database   string       `toml:"database"`
	LogLevel   logrus.Level `toml:"log_level"`

	Peer struct {
		TLSCert       string `toml:"tls_cert"`
		TLSKey        string `toml:"tls_key"`
		ListenServers []struct {
			Addr string `toml:"addr"`
			Upnp bool   `toml:"upnp"`
		} `toml:"listen_servers"`
		BootstrapPeers []string `toml:"bootstrap_peers"`
	} `toml:"peer"`

	Core struct {
		MaxConcurrentConn *uint     `toml:"max_concurrent_conn"`
		ListMessageTime   *duration `toml:"list_message_time"`
		ScanPeerTime      *duration `toml:"scan_peer_time"`
		PeerRetryTime     *duration `toml:"peer_retry_time"`
		EnableLpa         bool      `toml:"enable_lp_announcement"`
		EnableLpd         bool      `toml:"enable_lp_discovery"`
	} `toml:"core"`
}

func createDB() error {
	key, err := nymo.GenerateUser()
	if err != nil {
		return err
	}
	return createDatabase(config.Database, key)
}

func createTLSKeyPair() (e error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1)}
	cert, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		return err
	}
	certFile, err := os.Create(config.Peer.TLSCert)
	if err != nil {
		return err
	}
	defer func() {
		_ = certFile.Close()
		if e != nil {
			_ = os.Remove(certFile.Name())
		}
	}()

	err = pem.Encode(certFile, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})
	if err != nil {
		return err
	}

	keyFile, err := os.Create(config.Peer.TLSKey)
	if err != nil {
		return err
	}
	defer func() {
		_ = keyFile.Close()
		if e != nil {
			_ = os.Remove(keyFile.Name())
		}
	}()

	return pem.Encode(keyFile, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func getCoreConfig() *nymo.Config {
	cfg := nymo.DefaultConfig()
	if config.Core.MaxConcurrentConn != nil {
		cfg.MaxConcurrentConn = *config.Core.MaxConcurrentConn
	}
	if config.Core.ListMessageTime != nil {
		cfg.ListMessageTime = time.Duration(*config.Core.ListMessageTime)
	}
	if config.Core.ScanPeerTime != nil {
		cfg.ScanPeerTime = time.Duration(*config.Core.ScanPeerTime)
	}
	if config.Core.PeerRetryTime != nil {
		cfg.PeerRetryTime = time.Duration(*config.Core.PeerRetryTime)
	}
	cfg.LocalPeerAnnounce = config.Core.EnableLpa
	cfg.LocalPeerDiscover = config.Core.EnableLpd
	// XXX: writer no close
	cfg.Logger = stdlog.New(log.WriterLevel(logrus.ErrorLevel), "[core] ", 0)
	return cfg
}

func notExists(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func init() {
	s := flag.String("config", "config.toml", "config file path")
	flag.Parse()

	log.Formatter = &logrus.TextFormatter{
		ForceColors:            true,
		DisableTimestamp:       true,
		DisableLevelTruncation: true,
	}

	_, err := toml.DecodeFile(*s, &config)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(config.LogLevel)

	if notExists(config.Database) {
		log.Warn("[webui] database not found, creating a new one.")
		if err := createDB(); err != nil {
			log.Fatal(err)
		}
	}

	if notExists(config.Peer.TLSCert) || notExists(config.Peer.TLSKey) {
		log.Warn("[webui] TLS key pair not found, creating a new one.")
		if err := createTLSKeyPair(); err != nil {
			log.Fatal(err)
		}
	}
}
