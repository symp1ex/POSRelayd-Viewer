package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Message struct {
	Type      string                 `json:"type"`
	ClientID  string                 `json:"client_id,omitempty"`
	CommandID string                 `json:"command_id,omitempty"`
	Command   string                 `json:"command,omitempty"`
	Prompt    string                 `json:"prompt,omitempty"`
	Result    map[string]interface{} `json:"result,omitempty"`
	Role      string                 `json:"role,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Password  string                 `json:"password,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

func main() {
	server := "ws://10.127.33.42:22233/ws"

	var clientID string
	var password string

	fmt.Print("Введите id-подключения: ")
	fmt.Scan(&clientID)

	fmt.Print("Введите пароль: ")
	fmt.Scan(&password)

	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		panic(err)
	}

	adminID := uuid.NewString()

	// === AUTH ===
	conn.WriteJSON(Message{
		Type:     "auth",
		ClientID: clientID,
		Password: password,
	})

	var authResp Message
	if err := conn.ReadJSON(&authResp); err != nil {
		panic(err)
	}

	if authResp.Type != "auth_ok" {
		fmt.Println("Ошибка авторизации:", authResp.Error)
		return
	}

	fmt.Println("Авторизация успешна")

	// === REGISTER ===
	conn.WriteJSON(Message{
		Type: "register",
		Role: "admin",
		ID:   adminID,
	})

	go func() {
		for {
			var msg Message
			conn.ReadJSON(&msg)

			switch msg.Type {

			case "interactive_prompt":
				fmt.Print(msg.Prompt + " ")
				reader := bufio.NewReader(os.Stdin)
				answer, _ := reader.ReadString('\n')

				conn.WriteJSON(Message{
					Type:      "interactive_response",
					CommandID: msg.CommandID,
					Command:   answer[:len(answer)-1],
				})

			case "result":
				fmt.Println("\n=== OUTPUT ===")
				fmt.Println(msg.Result["output"])
				fmt.Println(msg.Result["prompt"])
			}
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		cmd, _ := reader.ReadString('\n')
		cmd = cmd[:len(cmd)-1]

		conn.WriteJSON(Message{
			Type:      "command",
			ClientID:  clientID,
			CommandID: uuid.NewString(),
			Command:   cmd,
		})
	}
}
