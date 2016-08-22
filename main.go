package main

import (
	"encoding/gob"
	"flag"
	"log"
	"net"
	"sync"

	"github.com/Bccp-Team/bccp-runner/endpoint"
	"github.com/Bccp-Team/bccp-runner/job"
	"github.com/Bccp-Team/bccp-server/message"
)

//FIXME refactor

type jobWrapper struct {
	currentJob *job.Job
	jobId      int
	mut        sync.Mutex
}

var (
	globatmut sync.Mutex
	jobs      map[int]*jobWrapper
)

func kill(encoder *gob.Encoder, id int) {
	globatmut.Lock()
	defer globatmut.Unlock()

	j, ok := jobs[id]
	answer := message.ClientRequest{}

	if !ok {
		answer.Kind = message.Error
		answer.Message = "No job to kill"
		encoder.Encode(&answer)
		return
	}

	err := j.currentJob.Kill("canceled")

	if err != nil {
		answer.Kind = message.Error
		answer.Message = err.Error()
		encoder.Encode(&answer)
		return
	}
}

func ping(encoder *gob.Encoder) {
	answer := message.ClientRequest{Kind: message.Ack}
	go encoder.Encode(&answer)
}

func run(servReq *message.ServerRequest, encoder *gob.Encoder) {

	globatmut.Lock()
	defer globatmut.Unlock()

	answer := message.ClientRequest{}
	_, ok := jobs[servReq.JobId]

	if ok {
		answer.Message = "The job is already running"
		answer.Kind = message.Error
		encoder.Encode(&answer)
		return
	}

	runReq := servReq.Run

	if runReq == nil || runReq.Init == "" || runReq.Repo == "" || runReq.Name == "" || runReq.UpdateTime == 0 || runReq.Timeout == 0 {
		answer.Message = "Missing some parameters"
		answer.Kind = message.Error
		encoder.Encode(&answer)
		return
	}

	j := &jobWrapper{}

	api := endpoint.NewApiWrapper(servReq.JobId, runReq.UpdateTime, encoder)
	j.currentJob = job.NewJob(runReq.Repo, runReq.Name, runReq.Init, runReq.Timeout, api)
	j.jobId = servReq.JobId

	jobs[servReq.JobId] = j

	go func() {
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			api.Push()
		}()

		j.currentJob.Run()
		wg.Wait()

		globatmut.Lock()
		defer globatmut.Unlock()
		delete(jobs, j.jobId)
	}()
}

func main() {
	var runnerName string
	var serverToken string
	var serverIp string
	var concurrency int
	flag.StringVar(&runnerName, "runner-name", "bccp runner", "the runner token")
	flag.StringVar(&serverToken, "runner-token", "bccp_token", "the runner token")
	flag.StringVar(&serverIp, "runner-service", "127.0.0.1:4243", "the runner service")
	flag.IntVar(&concurrency, "runner-concurrency", 7, "the runner capacity")

	flag.Parse()
	conn, err := net.Dial("tcp", serverIp)

	if err != nil {
		log.Panic(err)
	}

	encoder := gob.NewEncoder(conn)
	decoder := gob.NewDecoder(conn)

	jobs = make(map[int]*jobWrapper)

	request := message.SubscribeRequest{Token: serverToken, Concurrency: concurrency, Name: runnerName}
	answer := message.SubscribeAnswer{}

	err = encoder.Encode(&request)

	if err != nil {
		log.Panic(err)
	}

	err = decoder.Decode(&answer)

	if err != nil {
		log.Panic(err)
	}

	log.Printf("connected to server")

	for {
		servReq := message.ServerRequest{}
		err = decoder.Decode(&servReq)
		if err != nil {
			log.Panic(err)
		}
		switch servReq.Kind {
		case message.Ping:
			log.Printf("ping request")
			ping(encoder)
		case message.Kill:
			log.Printf("kill request")
			kill(encoder, servReq.JobId)
		case message.Run:
			log.Printf("run request")
			run(&servReq, encoder)
		}
	}
}
