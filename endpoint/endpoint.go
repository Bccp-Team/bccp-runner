package endpoint

import (
	"encoding/gob"
	"log"
	"time"

	"github.com/Bccp-Team/bccp-server/message"
)

type APIWrapper struct {
	jobID         int
	messageBuffer chan (string)
	finished      bool
	messages      []string
	status        string
	pushTimer     uint
	encoder       *gob.Encoder
}

func NewAPIWrapper(id int, pushTimer uint, encoder *gob.Encoder) *APIWrapper {
	var api APIWrapper

	api.jobID = id
	api.messageBuffer = make(chan string)
	api.messages = make([]string, 0, 10)
	api.finished = false
	api.pushTimer = pushTimer
	api.encoder = encoder

	return &api
}

func (api *APIWrapper) AppendOutput(message string) {
	api.messageBuffer <- message
}

func (api *APIWrapper) Finish(status string) {
	api.status = status
	api.finished = true
}

func (api *APIWrapper) Push() {
	tick := time.Tick(time.Second * time.Duration(api.pushTimer))

	for !api.finished {
		select {
		case message := <-api.messageBuffer:
			api.messages = append(api.messages, message)
		case <-tick:
			if len(api.messages) > 0 {
				oldmessages := api.messages
				api.messages = make([]string, 0, 10)
				api.pushResult(oldmessages) //go this method ?
			}
		}
	}

	//FIXME this code is maybe useless

	remaining := true

	for remaining {
		select {
		case message := <-api.messageBuffer:
			api.messages = append(api.messages, message)
		default:
			remaining = false
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
