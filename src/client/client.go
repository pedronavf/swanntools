package main

import (
	"crypto/x509"
	"crypto/tls"
	"io/ioutil"
	"strconv"
	"log"
	"github.com/jpillora/backoff"
	"time"
	"os"
)

const (
	socketBufferSize           = 1460
	SuccessfulClientAuthString = "200"
	FailedClientAuthString     = "403"
)

// client represents the local machine sending the DVR stream
type client struct {
	// conn is the TCP (w/ TLS) connection to the server
	conn *tls.Conn
	// send is the channel on which messages are sent
	send chan []byte
	// channel is the channel number of the stream
	channel *int
}

// Client makes a new client
func Client(channel *int) *client {
	return &client{
		conn:    newServerConnection(channel),
		send:    make(chan []byte, socketBufferSize),
		channel: channel,
	}
}

// Handle handles events such as messages being sent
func Handle(c *client) {
	for {
		select {
		case message := <-c.send:
			_, err := c.conn.Write(message)
			if err != nil {
				log.Println("Error occurred while reading from DVR stream connection: ", err.Error())
				log.Println("Attempting to reestablish connection...")
				c.conn.Close()
				c.conn = newServerConnection(c.channel)
				continue
			}
		}
	}
}

func newServerConnection(channel *int) *tls.Conn {
	// Load client key pair
	cert, err := tls.LoadX509KeyPair(config.certs+"/client.pem", config.certs+"/client.key")
	if err != nil {
		log.Fatalln("Unable to load client key pair")
	}

	// Read the server certificate
	serverCert, err := ioutil.ReadFile(config.certs + "/server.pem")
	if err != nil {
		log.Fatalln("Unable to read server certificate file")
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(serverCert)

	// Add both certificates to the TLS config
	tlsConfig := &tls.Config{
		// TODO: Generate CA-signed certificates instead
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{cert},
		RootCAs:            roots,
	}

	log.Println("Establishing connection and authenticating with server...")

	// Add a backoff/retry algorithm
	b := &backoff.Backoff{
		Min:    100 * time.Millisecond,
		Max:    5 * time.Minute,
		Factor: 2,
		Jitter: false,
	}
	var conn *tls.Conn
	for {
		// Create a new connection
		conn, err = tls.Dial("tcp", config.dest.String(), tlsConfig)
		if err != nil {
			d := b.Duration()
			log.Println("Unable to dial the server: ", err.Error())
			log.Printf("Retrying in %s...\n", d)
			time.Sleep(d)
			continue
		}

		// Send the channel number along with login details
		_, err = conn.Write([]byte(strconv.Itoa(*channel) + config.key + "\n"))
		if err != nil {
			conn.Close()
			d := b.Duration()
			log.Println("Writing stream init to server failed: ", err.Error())
			log.Printf("Retrying in %s...\n", d)
			time.Sleep(d)
			continue
		}

		b.Reset()
		break
	}

	// Verify the server response
	authResponse := make([]byte, 3)
	_, err = conn.Read(authResponse)
	if err != nil {
		conn.Close()
		log.Fatalln("Unable to read the server authentication response: ", err.Error())
	}

	if string(authResponse) == SuccessfulClientAuthString {
		log.Println("Successfully authenticated with the server.")
	} else if string(authResponse) == FailedClientAuthString {
		conn.Close()
		log.Fatalln("Authentication failed due to invalid credentials.")
		os.Exit(1)
	} else {
		conn.Close()
		log.Fatalln("Authentication failed due to unknown reason.")
		os.Exit(1)
	}

	return conn
}
