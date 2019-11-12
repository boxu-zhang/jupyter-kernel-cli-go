package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

func createKernel(url string, method string, lang string) string {
	body := bytes.NewBuffer([]byte(fmt.Sprintf("{\"name\": \"%s\"}", lang)))
	req, err := http.NewRequest("POST", url, body)

	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())

		return ""
	}

	req.Header.Add("auth_username", "fakeuser")
	req.Header.Add("auth_password", "fakepass")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())

		return ""
	}

	defer resp.Body.Close()

	data, _ := ioutil.ReadAll(resp.Body)
	response := make(map[string]interface{})

	err = json.Unmarshal(data, &response)

	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())

		return ""
	}

	kernelId, ok := response["id"]

	if !ok {
		fmt.Printf("Error: expecting a kernel id returned, but which is empty")
		fmt.Println(response)

		return ""
	}

	return kernelId.(string)
}

type Header struct {
	MessageUuid string `json:"msg_id"`
	Username    string `json:"username"`
	SessionUuid string `json:"session"`
	MessageType string `json:"msg_type"`
	Version     string `json:"version"`
}

type Message struct {
	Header       `json:"header,omitempty"`
	ParentHeader Header                 `json:"parent_header"`
	Metadata     map[string]interface{} `json:"metadata"`
	Content      map[string]interface{} `json:"content"`
	Buffers      []interface{}          `json:"buffers"`
	Channel      string                 `json:"channel"`
}

func main() {
	options := struct {
		code      *string
		lang      *string
		times     *int
		kernel_id *string
	}{
		code:      flag.String("code", "print('hello, world!')", "The code to execute on kernel."),
		lang:      flag.String("lang", "python", "The kernel language if a new kernel will be created."),
		times:     flag.Int("times", 100, "The number of times to execute the code string."),
		kernel_id: flag.String("kernel-id", "", "The id of an existing kernel for connecting and executing code. If not specified, a new kernel will be created."),
	}

	flag.Parse()

	if flag.Parsed() == false {
		flag.Usage()
	}

	base_url := os.Getenv("BASE_GATEWAY_HTTP_URL")

	if base_url == "" {
		base_url = "http://localhost:8888"
	}

	base_ws_url := os.Getenv("BASE_GATEWAY_WS_URL")

	if base_ws_url == "" {
		base_ws_url = "ws://localhost:8888"
	}

	fmt.Println(base_url)
	fmt.Println(base_ws_url)

	kernel_id := *options.kernel_id

	if *options.kernel_id == "" {
		kernel_id = createKernel(fmt.Sprintf("%s/api/kernels", base_url), "POST", *options.lang)

		if kernel_id == "" {
			return
		}

		fmt.Printf("Created kernel %s. Connect other clients with the following command:\n", kernel_id)
		fmt.Printf("\t docker-compose run client --kernel-id=%s\n", kernel_id)
	}

	ws, _, err := websocket.DefaultDialer.Dial(fmt.Sprintf("%s/api/kernels/%s/channels", base_ws_url, kernel_id), nil)

	if err != nil {
		fmt.Printf("Error: cannot connect to websocket because %s\n", err.Error())

		return
	}

	for i := 0; i < *options.times; i++ {
		fmt.Printf("Sending message %d of %d\n", i, *options.times)

		msg_id := uuid.New().String()

		msg := Message{
			Header: Header{
				Username:    "",
				Version:     "5.0",
				SessionUuid: "",
				MessageUuid: msg_id,
				MessageType: "execute_request",
			},
			Content: map[string]interface{}{
				"code":          *options.code,
				"silent":        false,
				"store_history": true,
				"allow_stdin":   false,
				"stop_on_error": true,
			},
			Channel: "shell",
		}

		data, err := json.Marshal(msg)

		if err != nil {
			fmt.Printf("Error, cannot marshal message, because %s\n", err.Error())
			return
		}

		err = ws.WriteMessage(websocket.TextMessage, data)

		if err != nil {
			fmt.Printf("Error, cannot WriteMessage because %s\n", err.Error())
			return
		}

		for {
			_, bytesReturn, err := ws.ReadMessage()

			if err != nil {
				fmt.Printf("Error, cannot ReadMessage due to %s\n", err.Error())
			}

			var response Message

			err = json.Unmarshal(bytesReturn, &response)

			if err != nil {
				fmt.Printf("Error, %s\n", err.Error())
				return
			}

			fmt.Printf("Recieved message type: %s\n", response.MessageType)

			if response.MessageType == "error" {
				fmt.Print("ERROR")
				fmt.Println(response)
				break
			}

			parent_msg_id := response.ParentHeader.MessageUuid

			if response.MessageType == "stream" && parent_msg_id == msg_id {
				fmt.Println("  Content: ", response.Content["text"].(string))
				break
			}
		}

		if ((i + 1) % 30) == 0 {
			time.Sleep(1)
		}
	}
}
