package job

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/bccp-runner/endpoint"
)

type Job struct {
	repo    string
	name    string
	init    string
	timeout uint
	api     *endpoint.ApiWrapper
	cmd     *exec.Cmd
}

func NewJob(repo, name, init string, timeout uint, api *endpoint.ApiWrapper) *Job {
	var job Job
	job.repo = repo
	job.name = name
	job.init = init
	job.timeout = timeout
	job.api = api
	return &job
}

func (job *Job) Kill() (err error) {
	err = job.cmd.Process.Signal(syscall.SIGTERM)
	return
}

func (job *Job) Run() {
	out, err := job.clonerepo()

	if err != nil {
		job.api.AppendOutput(out)
		job.api.Finish(1)
		return
	}

	err = ioutil.WriteFile(job.name+"/init.sh", []byte(job.init), 0644)

	if err != nil {
		job.api.AppendOutput(err.Error())
		job.api.Finish(1)
		return
	}

	job.exec()
}

func (job *Job) clonerepo() (out string, err error) {

	if _, err = os.Stat(job.name); err == nil {
		err = os.RemoveAll(job.name)
		if err != nil {
			out = err.Error()
			return
		}
	}

	rawoutput, err := exec.Command("git", "clone", job.repo, job.name).CombinedOutput()

	if err != nil {
		out = err.Error() + ":" + string(rawoutput)
	}
	return
}

func (job *Job) exec() (err error) {
	cmd := exec.Command("bash", "-c", "cd "+job.name+"; . init.sh")

	job.cmd = cmd

	var pipes [2]io.ReadCloser
	kind := [2]string{"out", "err"}
	pipes[0], err = cmd.StdoutPipe()

	if err != nil {
		job.api.AppendOutput("(error) " + err.Error())
		job.api.Finish(1)
		return
	}

	pipes[1], err = cmd.StderrPipe()

	if err != nil {
		job.api.AppendOutput("(error) " + err.Error())
		job.api.Finish(1)
		return
	}

	err = cmd.Start()

	if err != nil {
		job.api.AppendOutput("(error) " + err.Error())
		job.api.Finish(1)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	for i := 0; i < len(pipes); i = i + 1 {
		go func(i int) {
			defer wg.Done()
			scanner := bufio.NewScanner(pipes[i])

			for scanner.Scan() {
				job.api.AppendOutput("(" + kind[i] + ") " + scanner.Text())
			}
		}(i)
	}

	timer := time.NewTimer(time.Minute * time.Duration(job.timeout))
	finished := make(chan bool, 1)

	go func() {
		select {
		case <-timer.C:
			job.Kill()
		case <-finished:
		}
	}()

	err = cmd.Wait()
	finished <- true

	wg.Wait()

	if err == nil {
		job.api.Finish(0)
	} else { //FIXME: find a way to retreive errorcode as integer
		job.api.AppendOutput("(error) " + err.Error())
		job.api.Finish(1)
	}

	return
}
