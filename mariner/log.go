package mariner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	cwl "github.com/uc-cdis/cwl.go"
)

// TODO - write json encodings for all this AFTER implementing it
// so I don't constantly add/delete encodings

// MainLog is the interface for writing logs to workflowHistorydb
type MainLog struct {
	Path      string           `json:"-"` // path to log file to write/update
	Request   *WorkflowRequest `json:"request"`
	Main      *Log             `json:"main"`
	ByProcess map[string]*Log  `json:"byProcess"`
}

type awsCredentials struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

// for fetching sub-paths of a key, probably - https://docs.aws.amazon.com/sdk-for-go/api/service/s3/#S3.ListObjects

func newS3Session() (*session.Session, error) {
	secret := []byte(os.Getenv("AWSCREDS")) // probably make this a constant
	creds := &awsCredentials{}
	err := json.Unmarshal(secret, creds)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling aws secret: %v", err)
	}
	credsConfig := credentials.NewStaticCredentials(creds.ID, creds.Secret, "")
	awsConfig := &aws.Config{
		Region:      aws.String(Config.Storage.S3.Region),
		Credentials: credsConfig,
	}
	sess := session.Must(session.NewSession(awsConfig))
	return sess, nil
}

// TODO - sort list - latest to oldest request
func listRuns(userID string) ([]string, error) {
	sess, err := newS3Session()
	if err != nil {
		return nil, err
	}
	svc := s3.New(sess)
	prefix := fmt.Sprintf(pathToUserRunsf, userID)
	query := &s3.ListObjectsV2Input{
		Bucket:    aws.String(Config.Storage.S3.Name),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	}
	result, err := svc.ListObjectsV2(query)
	if err != nil {
		return nil, err
	}
	runIDs := []string{}
	for _, v := range result.CommonPrefixes {
		runID := strings.Split(v.String(), "/")[2]
		runIDs = append(runIDs, runID)
	}
	return runIDs, nil
}

// split this out into smaller, more atomic functions as soon as it's working - refactor
// most API endpoint handlers will call this function
func fetchMainLog(userID, runID string) (*MainLog, error) {
	sess, err := newS3Session()
	if err != nil {
		return nil, err
	}
	// Create a downloader with the session and default options
	downloader := s3manager.NewDownloader(sess)

	// Create a buffer to write the S3 Object contents to.
	// see: https://stackoverflow.com/questions/41645377/golang-s3-download-to-buffer-using-s3manager-downloader
	buf := &aws.WriteAtBuffer{}

	objKey := fmt.Sprintf(pathToUserRunLogf, userID, runID)

	// Write the contents of S3 Object to the buffer
	s3Obj := &s3.GetObjectInput{
		Bucket: aws.String(Config.Storage.S3.Name),
		Key:    aws.String(objKey),
	}
	_, err = downloader.Download(buf, s3Obj)
	if err != nil {
		return nil, fmt.Errorf("failed to download file, %v", err)
	}
	b := buf.Bytes()
	log := &MainLog{}
	err = json.Unmarshal(b, log)
	if err != nil {
		return nil, fmt.Errorf("error unmarhsalling log: %v", err)
	}
	return log, nil
}

func mainLog(path string, request *WorkflowRequest) *MainLog {
	log := &MainLog{
		Path:      path,
		Request:   request,
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

func (mainLog *MainLog) write() error {
	fmt.Println("writing main log..")

	// apply/update timestamps on the main log
	// not sure if I should collect timestamps of all writes
	// or just the times of first write and latest writes
	//
	// presently only applying timestamps at the top-level workflow (i.e., engine) level
	// NOT applying timestamps to individual tasks/subworkflows, but that would be easy to do
	// just not sure if it's useful/necessary right now

	// do this BY PROCESS
	// the "engine" corresponds to the top level workflow task
	// and should not be treated specially here
	/*
		t := ts()
		if log.Engine.Created == "" {
			log.Engine.Created = t
		}
		log.Engine.LastUpdated = t
	*/

	fmt.Println("marshalling MainLog to json..")
	j, err := json.Marshal(*mainLog)
	check(err)
	fmt.Println("writing data to file..")
	err = ioutil.WriteFile(mainLog.Path, j, 0644)
	check(err)
	return nil
}

// Log stores the eventLog and runtime stats for a mariner component (i.e., engine or task)
// see: https://golang.org/pkg/time/
type Log struct {
	Created        string                 `json:"created,omitempty"`     // okay - timezone???
	CreatedObj     time.Time              `json:"-"`                     // okay
	LastUpdated    string                 `json:"lastUpdated,omitempty"` // okay - timezone???
	LastUpdatedObj time.Time              `json:"-"`                     // okay
	JobID          string                 `json:"jobID,omitempty"`       // okay
	JobName        string                 `json:"jobName,omitempty"`     // keeping for now, but might be redundant w jobID
	Status         string                 `json:"status"`                // okay
	Stats          Stats                  `json:"stats"`                 // TODO
	Event          EventLog               `json:"eventLog,omitempty"`    // TODO
	Input          map[string]interface{} `json:"input"`                 // TODO for workflow; okay for task
	Output         cwl.Parameters         `json:"output"`                // okay
}

// called when a task is run
func (mainLog *MainLog) start(task *Task) {
	task.Log.start()
	mainLog.write()
}

// called when a task finishes running
func (mainLog *MainLog) finish(task *Task) {
	task.Log.finish()
	mainLog.write()
}

// called when a task finishes running
func (log *Log) finish() {
	t := time.Now()
	log.LastUpdatedObj = t
	log.LastUpdated = timef(log.LastUpdatedObj)
	log.Stats.DurationObj = t.Sub(log.CreatedObj)
	log.Stats.Duration = log.Stats.DurationObj.Minutes()
	log.Status = completed
}

// called when a task is run
func (log *Log) start() {
	t := time.Now()
	log.CreatedObj = t
	log.Created = timef(t)
	log.LastUpdatedObj = t
	log.LastUpdated = timef(t)
	log.Status = running
}

func logger() *Log {
	logger := &Log{
		Status: notStarted,
		Input:  make(map[string]interface{}),
	}
	return logger
}

// Stats holds performance stats for a given process
// recorded for tasks as well as workflows
// Runtime for a workflow is the sum of runtime of that workflow's steps
type Stats struct {
	CPU         ResourceStat  `json:"cpu"`       // in-progress
	Memory      ResourceStat  `json:"memory"`    // in-progress
	Duration    float64       `json:"duration"`  // okay - currently measured in minutes
	DurationObj time.Duration `json:"-"`         // okay
	NFailures   int           `json:"nfailures"` // TODO
	NRetries    int           `json:"nretries"`  // TODO
}

// ResourceStat is for logging resource requests vs. actual usage
type ResourceStat struct {
	Min    int64 `json:"min"`
	Max    int64 `json:"max"`
	Actual int64 `json:"actual,omitempty"`
}

// EventLog is an event logger for a mariner component (i.e., engine or task)
type EventLog []string

// a record is "<timestamp> - <level> - <message>"
func (log EventLog) write(level, message string) {
	timestamp := ts()
	// timezone???
	record := fmt.Sprintf("%v - %v - %v", timestamp, level, message)
	log = append(log, record)
}

func (log *EventLog) infof(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.info(m)
}

func (log *EventLog) info(m string) {
	log.write(infoLogLevel, m)
}

func (log *EventLog) warnf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.warn(m)
}

func (log *EventLog) warn(m string) {
	log.write(warningLogLevel, m)
}

func (log *EventLog) errorf(f string, v ...interface{}) {
	m := fmt.Sprintf(f, v...)
	log.error(m)
}

func (log *EventLog) error(m string) {
	log.write(errorLogLevel, m)
}

// get string timestamp for right now
func ts() string {
	t := time.Now()
	s := timef(t)
	return s
}

// TODO
// should just have a function that returns (t time.Time, ts string)
// ---> return the time object AND the string timestamp for right now

// convert time object to string timestamp
// for logging purposes
// TODO timezone???
func timef(t time.Time) string {
	s := fmt.Sprintf("%v/%v/%v %v:%v:%v", t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute(), t.Second())
	return s
}
