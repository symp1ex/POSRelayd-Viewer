package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

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

// ===== ВВОД ПАРОЛЯ =====

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	reader := bufio.NewReader(os.Stdin)
	text, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(text, "\r\n"), nil
}

// ===== АВТОРИЗАЦИЯ =====

func authLoop(conn *websocket.Conn, reader *bufio.Reader) string {
	for {
		fmt.Print("Введите id-подключения: ")
		clientID, err := reader.ReadString('\n')
		if err != nil {
			continue
		}
		clientID = strings.TrimSpace(clientID)

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

// ===== ПОДКЛЮЧЕНИЕ С RETRY =====

func connectWithRetry(server string) *websocket.Conn {
	for {
		conn, _, err := websocket.DefaultDialer.Dial(server, nil)
		if err != nil {
			fmt.Println("Сервер недоступен, повторная попытка через 10 секунд...")
			time.Sleep(10 * time.Second)
			continue
		}
		fmt.Println("Соединение с сервером установлено")
		return conn
	}
}

// ===== MAIN =====

func main() {
	server := "ws://10.127.33.42:22233/ws"
	reader := bufio.NewReader(os.Stdin)

	for {
		// ===== ПОДКЛЮЧЕНИЕ С ПОВТОРАМИ =====
		conn := connectWithRetry(server)

		clientID := authLoop(conn, reader)
		adminID := uuid.NewString()

		_ = conn.WriteJSON(Message{
			Type: "register",
			Role: "admin",
			ID:   adminID,
		})

		sessionClosed := make(chan struct{})
		inputCh := make(chan string)
		stopInput := make(chan struct{})

		// ===== ЕДИНСТВЕННЫЙ stdin-reader =====
		go func() {
			defer close(inputCh)
			for {
				select {
				case <-stopInput:
					return
				default:
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					inputCh <- strings.TrimRight(line, "\r\n")
				}
			}
		}()

		// ===== ЧТЕНИЕ СЕРВЕРА =====
		go func() {
			defer close(sessionClosed)

			for {
				var msg Message
				if err := conn.ReadJSON(&msg); err != nil {
					fmt.Println("\nСоединение разорвано")
					return
				}

				switch msg.Type {

				case "interactive_prompt":
					fmt.Print(msg.Prompt)

					_ = conn.WriteJSON(Message{
						Type:      "interactive_response",
						CommandID: msg.CommandID,
						Command:   "",
						ID:        adminID,
					})

				case "result":
					if out, ok := msg.Result["output"].(string); ok {
						fmt.Print(out)
					}

				case "session_closed":
					fmt.Println("\nСессия клиента завершена")
					return
				}
			}
		}()

		// ===== ОСНОВНОЙ ЦИКЛ =====
		for {
			select {
			case <-sessionClosed:
				close(stopInput)
				conn.Close()
				fmt.Println("\nПереподключение к серверу...\n")
				goto RECONNECT

			case cmd, ok := <-inputCh:
				if !ok {
					continue
				}

				_ = conn.WriteJSON(Message{
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
