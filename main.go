package main

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	Port            string  = "7000"
	TimeoutDuration float64 = 5.0
	MessageDebounce float64 = 1.0
)

type MessageType int

const (
	ClientConnected MessageType = iota + 1
	ClientDisconnected
	NewMessage
)

type Message struct {
	Type MessageType
	Conn net.Conn
	Text string
}

type Client struct {
	Conn        net.Conn
	LastMessage time.Time
	TimedOut    bool
	Strikes     int
}

func server(messages chan Message) {
	clients := map[string]*Client{}
	bannedClients := map[string]bool{}

	for {
		message := <-messages

		switch message.Type {
		case ClientConnected:
			addr := message.Conn.RemoteAddr().String()
			_, isBanned := bannedClients[addr]

			if isBanned {
				message.Conn.Close()
				break
			}

			clients[addr] = &Client{
				Conn: message.Conn,
			}

			message.Conn.Write([]byte("Welcome to the box"))
			log.Printf("Client %s connected", addr)

		case ClientDisconnected:
			addr := message.Conn.RemoteAddr().String()
			delete(clients, addr)
			log.Printf("Client %s disconnected", addr)

		case NewMessage:
			addr := message.Conn.RemoteAddr().String()
			client := clients[addr]

			if client == nil {
				message.Conn.Close()
				log.Printf("Error: client with address: %s does not exist", addr)
				break
			}

			now := time.Now()
			passedTime := now.Sub(client.LastMessage)

			if client.TimedOut {
				if passedTime.Seconds() >= TimeoutDuration {
					log.Printf("Info: unban client %s", addr)
					client.TimedOut = false
				} else {
					log.Printf("Info: client %s still banned", addr)
					waitTimeString := fmt.Sprintf("%f", TimeoutDuration-passedTime.Seconds())
					client.Conn.Write([]byte("You are timed out. Chill for " + waitTimeString + " Seconds"))
					break
				}
			}

			if passedTime.Seconds() < MessageDebounce {
				client.Strikes += 1

				if client.Strikes >= 3 {
					bannedClients[addr] = true
					client.Conn.Write([]byte("you are banned. bye bye"))
					client.Conn.Close()
					delete(clients, addr)

					break
				}

				log.Printf("Info: timeout client %s for %f. number strikes: %d", addr, TimeoutDuration, client.Strikes)
				client.TimedOut = true
				client.LastMessage = time.Now()
				client.Conn.Write([]byte("Strike number " + strconv.Itoa(client.Strikes) + ". You are timed out. Wait for " + strconv.Itoa(int(TimeoutDuration))))
				break
			}

			client.LastMessage = time.Now()

			log.Printf("Info: send message %s by %s", message.Text, addr)
			for _, client := range clients {
				client.Conn.Write([]byte(message.Text))
			}
		}
	}
}

func handleClient(conn net.Conn, messages chan Message) {
	buffer := make([]byte, 256)
	for {
		bytes, err := conn.Read(buffer)

		if err != nil && err != io.EOF {
			slog.Error("Error in reading client message", "error", err)
			conn.Close()
			messages <- Message{
				Type: ClientDisconnected,
				Conn: conn,
			}
			return
		}

		if err != nil && err == io.EOF {
			conn.Close()
			messages <- Message{
				Type: ClientDisconnected,
				Conn: conn,
			}

			return
		}

		text := string(buffer[:bytes])
		messages <- Message{
			Type: NewMessage,
			Conn: conn,
			Text: text,
		}
	}
}

func main() {
	ln, err := net.Listen("tcp", ":"+Port)
	if err != nil {
		slog.Error("Create tcp conn", "error", err)
		os.Exit(0)
	}

	defer ln.Close()

	log.Printf("Listening to TCP connection on Port %s", Port)

	messages := make(chan Message)
	go server(messages)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("Accept conn", "error", err)
			continue
		}

		messages <- Message{
			Type: ClientConnected,
			Conn: conn,
		}

		go handleClient(conn, messages)
	}
}
