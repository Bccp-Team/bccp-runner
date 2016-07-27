package endpoint

import (
	"bccp-server/runners"
	"encoding/gob"
	"time"
)

type ApiWrapper struct {
	jobId         uint
	messageBuffer chan (string)
	finished      bool
	messages      []string
	errorcode     int
	pushTimer     uint
	encoder       *gob.Encoder
}

func NewApiWrapper(id uint, pushTimer uint, encoder *gob.Encoder) *ApiWrapper {
	var api ApiWrapper

	api.jobId = id
	api.messageBuffer = make(chan string)
	api.messages = make([]string, 0, 10)
	api.errorcode = 0
	api.finished = false
	api.pushTimer = pushTimer

	return &api
}

func (api *ApiWrapper) AppendOutput(message string) {
	api.messageBuffer <- message
}

func (api *ApiWrapper) Finish(errorcode int) {
	api.errorcode = errorcode
	api.finished = true
}

func (api *ApiWrapper) Push() {
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

func (api *ApiWrapper) pushResult(messages []string) {
	request := &runners.ClientRequest{Logs: messages, Kind: runners.Logs, JobId: api.jobId}
	api.encoder.Encode(request)
}

func (api *ApiWrapper) pushExitCode() {
	request := &runners.ClientRequest{Kind: runners.Finish, ReturnValue: api.errorcode, JobId: api.jobId}
	api.encoder.Encode(request)
}
