package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	cwl "github.com/uc-cdis/cwl.go"
)

// TODO - write json encodings for all this AFTER implementing it
// so I don't constantly add/delete encodings

// MainLog is the interface for writing logs to workflowHistorydb
type MainLog struct {
	Path      string           `json:"-"` // path to log file to write/update
	Request   *WorkflowRequest `json:"request"`
	Status    string           `json:"status"` // for API, 'runs/{runID}/status' and 'runs/{runID}/restart'
	Engine    *Log             `json:"engineLog"`
	ByProcess map[string]*Log  `json:"byProcess"`
}

func mainLog(path string, request *WorkflowRequest) *MainLog {
	log := &MainLog{
		Path:      path,
		Request:   request,
		Status:    IN_PROGRESS,
		Engine:    &Log{},
		ByProcess: make(map[string]*Log),
	}
	return log
}

func showLog(path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("error opening log: %v\n", err)
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Printf("error reading log: %v\n", err)
	}
	j := &MainLog{}
	err = json.Unmarshal(b, j)
	if err != nil {
		fmt.Printf("error unmarshalling log: %v\n", err)
	}
	printJSON(j)
}

// tmp, for debugging mostly, though could/should adapt for complete error handling interface
func check(err error) {
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func (log *MainLog) write() error {
	fmt.Println("writing main log..")
	fmt.Println("marshalling MainLog to json..")
	j, err := json.Marshal(*log)
	check(err)
	fmt.Println("writing data to file..")
	err = ioutil.WriteFile(log.Path, j, 0644)
	check(err)
	/*
		fmt.Println("creating MainLog file..")
		f, err := os.Create(log.Path)
		if err != nil {
			fmt.Printf("error: %v", err)
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			fmt.Printf("error: %v", err)
			return err
		}
	*/
	return nil
}

// Log stores the eventLog and runtime stats for a mariner component (i.e., engine or task)
type Log struct {
	Status string         `json:"status"`
	Stats  Stats          `json:"stats"`
	Event  EventLog       `json:"eventLog,omitempty"`
	Input  cwl.Parameters `json:"input"`
	Output cwl.Parameters `json:"output"`
}

func logger() *Log {
	logger := &Log{
		Status: NOT_STARTED,
	}
	return logger
}

// Stats holds performance stats for a given process
// recorded for tasks as well as workflows
// Runtime for a workflow is the sum of runtime of that workflow's steps
type Stats struct {
	CPU       int `json:"cpu"`
	Memory    int `json:"memory"`
	Duration  int `json:"duration"`
	NFailures int `json:"nfailures"`
	NRetries  int `json:"nretries"`
}

// EventLog is an event logger for a mariner component (i.e., engine or task)
type EventLog []string

// a record is "<timestamp> - <level> - <message>"
func (log EventLog) write(level, message string) {
	timestamp := ts()
	record := fmt.Sprintf("%v - %v - %v", timestamp, level, message)
	log = append(log, record)
}

func (log *EventLog) infof(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.info(m)
}

func (log *EventLog) info(m string) {
	log.write(INFO, m)
}

func (log *EventLog) warnf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.warn(m)
}

func (log *EventLog) warn(m string) {
	log.write(WARNING, m)
}

func (log *EventLog) errorf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.error(m)
}

func (log *EventLog) error(m string) {
	log.write(ERROR, m)
}

func ts() string {
	t := time.Now()
	s := fmt.Sprintf("%v/%v/%v %v:%v:%v", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
	return s
}
