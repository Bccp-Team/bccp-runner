package main

import (
	"encoding/gob"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"net"
	"sync"

	"github.com/Bccp-Team/bccp-runner/endpoint"
	"github.com/Bccp-Team/bccp-runner/job"
	"github.com/Bccp-Team/bccp-server/message"
)

//FIXME refactor

type jobWrapper struct {
	currentJob *job.Job
	jobID      int64
	mut        sync.Mutex
}

var (
	globalMut sync.Mutex
	jobs      map[int64]*jobWrapper
)

func kill(encoder *gob.Encoder, id int64) {
	globalMut.Lock()
	defer globalMut.Unlock()

	answer := message.ClientRequest{}

	j, ok := jobs[id]
	if !ok || j == nil {
		answer.Kind = message.Error
		answer.Message = "No job to kill"
		encoder.Encode(&answer)
		return
	}

	current := j.currentJob

	if current == nil {
		return
	}

	err := current.Kill("canceled")
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
	globalMut.Lock()
	defer globalMut.Unlock()

	answer := message.ClientRequest{}
	_, ok := jobs[servReq.JobID]
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

	api := endpoint.NewAPIWrapper(servReq.JobID, runReq.UpdateTime, encoder)
	j.currentJob = job.NewJob(runReq.Repo, runReq.Name, runReq.Init, runReq.Timeout, api)
	j.jobID = servReq.JobID

	jobs[servReq.JobID] = j

	go func() {
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			api.Push()
		}()

		j.currentJob.Run()
		wg.Wait()

		globalMut.Lock()
		defer globalMut.Unlock()

		delete(jobs, j.jobID)
	}()
}

func main() {
	var (
		runnerName  string
		serverToken string
		serverIP    string
		concurrency int64
	)

	sigs := make(chan os.Signal, 1)

	flag.StringVar(&runnerName, "runner-name", "bccp runner", "the runner token")
	flag.StringVar(&serverToken, "runner-token", "bccp_token", "the runner token")
	flag.StringVar(&serverIP, "runner-service", "127.0.0.1:4243", "the runner service")
	flag.Int64Var(&concurrency, "runner-concurrency", 7, "the runner capacity")
	flag.Parse()

	conn, err := net.Dial("tcp", serverIP)
	if err != nil {
		log.Panic(err)
	}

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigs
		conn.Close()
		os.Exit(0)
	}()

	encoder := gob.NewEncoder(conn)
	decoder := gob.NewDecoder(conn)

	jobs = make(map[int64]*jobWrapper)

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

	log.Print("connected to server")

	for {
		servReq := message.ServerRequest{}
		err = decoder.Decode(&servReq)
		if err != nil {
			log.Panic(err)
		}

		switch servReq.Kind {
		case message.Ping:
			log.Print("ping request")
			ping(encoder)
		case message.Kill:
			log.Print("kill request")
			kill(encoder, servReq.JobID)
		case message.Run:
			log.Print("run request")
			run(&servReq, encoder)
		}
	}
}
