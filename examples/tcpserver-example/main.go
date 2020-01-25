package main

import (
	"fmt"
	"log"
	"net"
)

const (
	connectionHost = "localhost"
	connectionPort = "12345"
	connectionType = "tcp"
)

func main() {
	srv, err := net.Listen(connectionType, fmt.Sprintf("%s:%s", connectionHost, connectionPort))
	if err != nil {
		log.Fatalf("Error starting listner: %v", err.Error())
	}
	defer srv.Close()

	log.Printf("Started listening on %s:%s", connectionHost, connectionPort)

	for {
		con, err := srv.Accept()
		if err != nil {
			log.Fatalf("Error while accepting connection: %v", err.Error())
		}
		go handleReq(con)
	}
}

func handleReq(conn net.Conn) {
	buff := make([]byte, 1024)

	reqLength, err := conn.Read(buff)
	if err != nil {
		log.Printf("Error while reading received data: %v", err.Error())
	}
	log.Printf("Received: %v", string(buff[:reqLength]))

	conn.Write([]byte("ACK\n"))
	conn.Close()
}
