package main

import (
	"net"
	"fmt"
	"encoding/hex"
	"os"
	"bytes"
	"log"
	"strconv"
	"math"
)

// Hex values of the byte arrays required in string form
const (
	initStreamValues     = "00000000000000000000010000000300000000000000000000006800000001000000100000000N0000000100000000UUUUUUUUUU000000000000000000000000000001000000000000010124000000PPPPPPPPPPPP00009cc9c805000000000400010004000000a8c9c80500000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	successfulAuthValues = "1000000000000000"
	failedAuthValues     = "0800000004000000"
)

// newStreamConnection creates and sets up a new TCP connection
func newStreamConnection(channel *int) *net.TCPConn {
	// Get the stream initialization bytes
	streamInitData := initStreamBytes(channel)

	// Set up a new connection
	conn, err := net.DialTCP("tcp", nil, config.source)
	if err != nil {
		log.Fatalln("Dialing the DVR failed:", err.Error())
	}

	// Send the stream initialization byte array
	fmt.Fprintln(os.Stdout, "Sending stream initialization byte array.")
	_, err = conn.Write(streamInitData)
	if err != nil {
		conn.Close()
		log.Fatalln("Writing stream init to DVR failed: ", err.Error())
	}

	// Check authentication response
	data := make([]byte, 8)
	_, err = conn.Read(data)
	if err != nil {
		conn.Close()
		log.Fatalln("Unable to read DVR authentication response: ", err.Error())
	}

	// Check if authenticated
	successfulAuthBytes, _ := hex.DecodeString(successfulAuthValues)
	failedAuthBytes, _ := hex.DecodeString(failedAuthValues)
	if bytes.Equal(data, successfulAuthBytes) {
		conn.Close()
		log.Fatalln("DVR authentication successful.")
	} else if bytes.Equal(data, failedAuthBytes) {
		conn.Close()
		log.Fatalln("DVR authentication failed due to invalid credentials.")
	} else {
		conn.Close()
		log.Fatalln("DVR authentication failed due to unknown reason.")
	}

	// Return the stream
	return conn
}

// initStreamBytes returns the byte array required to initialize a stream
func initStreamBytes(channel *int) []byte {
	hexValues := initStreamValues

	channelStartPos := 77
	channelEndPos := 78
	
	// Convert channel from 1, 2, 3, 4 to 1, 2, 4, 8 respectively
	parsedChannel := int(math.Exp2(float64(*channel - 1)))
	hexValues = hexValues[:channelStartPos] + fmt.Sprintf("%d", parsedChannel) + hexValues[channelEndPos:]

	// Parse username
	for i, v := range config.user {
		startPos := 94 + 2*i
		endPos := 96 + 2*i
		hexValues = hexValues[:startPos] + fmt.Sprintf("%x", int(v)) + hexValues[endPos:]
	}

	// Parse password
	for i, v := range config.pass {
		startPos := 158 + 2*i
		endPos := 160 + 2*i
		hexValues = hexValues[:startPos] + fmt.Sprintf("%x", int(v)) + hexValues[endPos:]
	}

	// Decode the hex string into a byte array
	byteArray, err := hex.DecodeString(hexValues)
	if err != nil {
		log.Fatalln("Unable to decode stream initialization values to byte array: ", err.Error())
	}
	return byteArray
}

// StreamToServer streams the video to the server
func StreamToServer(channel *int) {
	defer wg.Done()

	conn := newStreamConnection(channel)
	defer conn.Close()

	// Create a client
	c := Client(channel)

	// Start the client handler to receive messages
	go Handle(c)

	// Get the main camera stream
	for {
		data := make([]byte, socketBufferSize)
		n, err := conn.Read(data)
		if err != nil {
			log.Panicln("Error occurred while reading from DVR stream connection", err.Error())
		}

		c.send <- data[:n]
	}
}

func StreamToStdout(channel *int) {
	defer wg.Done()

	conn := newStreamConnection(channel)
	defer conn.Close()

	for {
		data := make([]byte, socketBufferSize)
		n, err := conn.Read(data)
		if err != nil {
			log.Panicln("Error occurred while reading from DVR stream connection", err.Error())
		}

		fmt.Fprintf(os.Stdout, "%s", data[:n])
	}
}

func StreamToFile(channel *int) {
	defer wg.Done()

	// Attempt to create output file
	fileName := "swann_" + strconv.Itoa(*channel)
	file, err := os.Create(fileName)
	if err != nil {
		log.Fatalln("Error occured while attempting to open output file", err.Error())
	}

	// Open file for writing
	file, err = os.OpenFile(fileName, os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	print("Writing to: " + fileName)
	defer file.Close()

	conn := newStreamConnection(channel)
	defer conn.Close()

	for {
		data := make([]byte, socketBufferSize)
		n, err := conn.Read(data)
		if err != nil {
			log.Panicln("Error occurred while reading from DVR stream connection", err.Error())
		}

		file.Write(data[:n])
	}
}
