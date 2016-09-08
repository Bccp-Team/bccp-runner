package endpoint

import (
	"encoding/gob"
	"log"
	"time"

	"github.com/Bccp-Team/bccp-server/message"
)

type APIWrapper struct {
	jobID         int64
	messageBuffer chan (string)
	messages      []string
	status        string
	pushTimer     uint64
	encoder       *gob.Encoder
}

func NewAPIWrapper(id int64, pushTimer uint64, encoder *gob.Encoder) *APIWrapper {
	var api APIWrapper

	api.jobID = id
	api.messageBuffer = make(chan string)
	api.messages = make([]string, 0, 10)
	api.pushTimer = pushTimer
	api.encoder = encoder

	return &api
}

func (api *APIWrapper) AppendOutput(message string) {
	api.messageBuffer <- message
}

func (api *APIWrapper) Finish(status string) {
	api.status = status
	close(api.messageBuffer)
}

func (api *APIWrapper) Push() {
	tick := time.Tick(time.Second * time.Duration(api.pushTimer))

	loop := true

	for loop {
		select {
		case message, ok := <-api.messageBuffer:
			if !ok {
				log.Printf("close buffer")
				loop = false
			} else {
				api.messages = append(api.messages, message)
			}
		case <-tick:
			if len(api.messages) > 0 {
				oldmessages := api.messages
				api.messages = make([]string, 0, 10)
				api.pushResult(oldmessages) //go this method ?
			}
		}
	}

	if len(api.messages) > 0 {
		api.pushResult(api.messages)
	}

	api.pushExitCode()
}

func (api *APIWrapper) pushResult(messages []string) {
	request := &message.ClientRequest{Logs: messages, Kind: message.Logs, JobID: api.jobID}
	log.Printf("push result")
	api.encoder.Encode(request)
}

func (api *APIWrapper) pushExitCode() {
	log.Printf("finish")
	request := &message.ClientRequest{Kind: message.Finish, Status: api.status, JobID: api.jobID}
	api.encoder.Encode(request)
}
