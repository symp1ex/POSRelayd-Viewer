package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/term"
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

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Если stdin — терминал, читаем скрыто
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	// Иначе (IDE, debug, pipe) — обычный ввод
	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return text[:len(text)-1], nil
}

func authLoop(conn *websocket.Conn) string {
	for {
		var clientID string

		fmt.Print("Введите id-подключения: ")
		fmt.Scan(&clientID)

		password, err := readPassword("Введите пароль: ")
		if err != nil {
			fmt.Println("Ошибка ввода пароля:", err)
			continue
		}

		if err := conn.WriteJSON(Message{
			Type:     "auth",
			ClientID: clientID,
			Password: password,
		}); err != nil {
			fmt.Println("Ошибка отправки:", err)
			continue
		}

		var resp Message
		if err := conn.ReadJSON(&resp); err != nil {
			fmt.Println("Ошибка сервера:", err)
			continue
		}

		if resp.Type == "auth_ok" {
			fmt.Println("Авторизация успешна")
			return clientID
		}

		fmt.Println("Ошибка авторизации:", resp.Error)
	}
}

func main() {
	server := "ws://10.127.33.42:22233/ws"

	for {
		conn, _, err := websocket.DefaultDialer.Dial(server, nil)
		if err != nil {
			fmt.Println("Ошибка подключения:", err)
			return
		}

		clientID := authLoop(conn)
		adminID := uuid.NewString()

		conn.WriteJSON(Message{
			Type: "register",
			Role: "admin",
			ID:   adminID,
		})

		sessionClosed := make(chan struct{})
		inputCh := make(chan string)

		// ===== ЕДИНСТВЕННЫЙ stdin reader =====
		go func() {
			reader := bufio.NewReader(os.Stdin)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					close(inputCh)
					return
				}
				inputCh <- strings.TrimRight(line, "\r\n")
			}
		}()

		// ===== ЧТЕНИЕ СЕРВЕРА =====
		go func() {
			defer close(sessionClosed)

			for {
				var msg Message
				if err := conn.ReadJSON(&msg); err != nil {
					fmt.Println("\nСоединение разорвано, нажмите Enter для продолжения")
					return
				}

				switch msg.Type {

				case "interactive_prompt":
					fmt.Print(msg.Prompt + " ")
					answer := <-inputCh

					conn.WriteJSON(Message{
						Type:      "interactive_response",
						CommandID: msg.CommandID,
						Command:   answer,
						ID:        adminID,
					})

				case "result":
					fmt.Println("\n=== OUTPUT ===")
					fmt.Println(msg.Result["output"])
					fmt.Println(msg.Result["prompt"])

				case "session_closed":
					fmt.Println("\nСессия клиента завершена, нажмите Enter для продолжения")
					return
				}
			}
		}()

		// ===== ОСНОВНОЙ ЦИКЛ =====
		for {
			select {
			case <-sessionClosed:
				conn.Close()
				fmt.Println("\nВозврат к выбору подключения...\n")
				goto RECONNECT

			default:
				fmt.Print("> ")
				cmd := <-inputCh

				conn.WriteJSON(Message{
					Type:      "command",
					ClientID:  clientID,
					CommandID: uuid.NewString(),
					Command:   cmd,
					ID:        adminID,
				})
			}
		}

	RECONNECT:
		continue
	}
}
