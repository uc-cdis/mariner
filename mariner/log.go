package mariner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// TODO - write json encodings for all this AFTER implementing it
// so I don't constantly add/delete encodings

// MainLog is the interface for writing logs to workflowHistorydb
type MainLog struct {
	sync.RWMutex `json:"-"`
	Path         string           `json:"path"` // tentative  - maybe can't write this - path to log file to write/update
	Request      *WorkflowRequest `json:"request"`
	Main         *Log             `json:"main"`
	ByProcess    map[string]*Log  `json:"byProcess"`
}

// MainLogJSON gets written to workflowHistorydb
type MainLogJSON struct {
	Path      string           `json:"path"` // tentative  - maybe can't write this - path to log file to write/update
	Request   *WorkflowRequest `json:"request"`
	Main      *Log             `json:"main"`
	ByProcess map[string]*Log  `json:"byProcess"`
}

// TODO - sort list - latest to oldest request
func (server *Server) listRuns(userID string) ([]string, error) {
	sess := server.S3FileManager.newS3Session()
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
func (server *Server) fetchMainLog(userID, runID string) (*MainLog, error) {
	var err error

	sess := server.S3FileManager.newS3Session()

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

func mainLog(path string) *MainLog {
	log := &MainLog{
		Path:      path,
		Main:      logger(),
		ByProcess: make(map[string]*Log),
	}
	return log
}

func (engine *K8sEngine) writeLogToS3() error {
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

	engine.Log.RLock()
	defer engine.Log.RUnlock()

	sess := engine.S3FileManager.newS3Session()
	uploader := s3manager.NewUploader(sess)

	mainLogJSON := MainLogJSON{
		Path:      engine.Log.Path,
		Request:   engine.Log.Request,
		Main:      engine.Log.Main,
		ByProcess: engine.Log.ByProcess,
	}
	j, err := json.Marshal(mainLogJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal log to json: %v", err)
	}

	objKey := fmt.Sprintf(pathToUserRunLogf, engine.UserID, engine.RunID)

	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(Config.Storage.S3.Name),
		Key:    aws.String(objKey),
		Body:   bytes.NewReader(j),
	})
	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}

	return nil
}

func (server *Server) writeLog(mainLog *MainLog, userID string, runID string) error {
	sess := server.S3FileManager.newS3Session()
	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	mainLogJSON := MainLogJSON{
		Path:      mainLog.Path,
		Request:   mainLog.Request,
		Main:      mainLog.Main,
		ByProcess: mainLog.ByProcess,
	}
	j, err := json.Marshal(mainLogJSON)
	if err != nil {
		return fmt.Errorf("failed to marshal log to json: %v", err)
	}

	objKey := fmt.Sprintf(pathToUserRunLogf, userID, runID)

	// Upload the file to S3.
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(Config.Storage.S3.Name),
		Key:    aws.String(objKey),
		Body:   bytes.NewReader(j),
	})

	if err != nil {
		return fmt.Errorf("failed to upload file, %v", err)
	}

	return nil
}

// Log stores the eventLog and runtime stats for a mariner component (i.e., engine or task)
// see: https://golang.org/pkg/time/
//
// note: could log the job spec per task
// ----- or at least the task container spec
// ----- need to double check there is no sensitive info in the job spec
// ----- should be fine
// ----- for now: log container image pulled for task
type Log struct {
	Created        string                 `json:"created,omitempty"` // timezone???
	CreatedObj     time.Time              `json:"-"`
	LastUpdated    string                 `json:"lastUpdated,omitempty"` // timezone???
	LastUpdatedObj time.Time              `json:"-"`
	JobID          string                 `json:"jobID,omitempty"`
	JobName        string                 `json:"jobName,omitempty"`
	ContainerImage string                 `json:"containerImage,omitempty"`
	Status         string                 `json:"status"`
	Stats          *Stats                 `json:"stats"`
	Event          *EventLog              `json:"eventLog,omitempty"`
	Input          map[string]interface{} `json:"input"`
	Output         map[string]interface{} `json:"output"`
	Scatter        map[int]*Log           `json:"scatter,omitempty"`
}

func (r *ResourceUsage) init() {
	r.Series = ResourceUsageSeries{}         // #race #ok
	r.SamplingPeriod = metricsSamplingPeriod // #race #ok
}

func (s *ResourceUsageSeries) append(p ResourceUsageSamplePoint) {
	*s = append(*s, p)
}

// called when a task is run
func (engine *K8sEngine) startTaskLog(task *Task) {
	task.Log.start()
	engine.writeLogToS3()
}

// called when a task finishes running
func (engine *K8sEngine) finishTaskLog(task *Task) {
	task.Log.finish()
	engine.writeLogToS3()
}

// called when a task finishes running
func (log *Log) finish() {
	t := time.Now()
	log.LastUpdatedObj = t
	log.LastUpdated = timef(log.LastUpdatedObj)
	log.Stats.DurationObj = t.Sub(log.CreatedObj)
	log.Stats.Duration = log.Stats.DurationObj.Seconds()
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
		Stats:  &Stats{},
		Event:  &EventLog{},
	}
	logger.Event.info("init log")
	return logger
}

// Stats holds performance stats for a given process
// recorded for tasks as well as workflows
// Runtime for a workflow is the sum of runtime of that workflow's steps
type Stats struct {
	CPUReq        ResourceRequirement `json:"cpuReq"` // in-progress
	MemoryReq     ResourceRequirement `json:"memReq"` // in-progress
	ResourceUsage ResourceUsage       `json:"resourceUsage"`
	Duration      float64             `json:"duration"`  // okay - currently measured in minutes
	DurationObj   time.Duration       `json:"-"`         // okay
	NFailures     int                 `json:"nfailures"` // TODO
	NRetries      int                 `json:"nretries"`  // TODO
}

// ResourceRequirement is for logging resource requests vs. actual usage
type ResourceRequirement struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

// ResourceUsage ..
type ResourceUsage struct {
	Series         ResourceUsageSeries `json:"data"`
	SamplingPeriod int                 `json:"samplingPeriod"`
}

// ResourceUsageSeries ..
type ResourceUsageSeries []ResourceUsageSamplePoint

// ResourceUsageSamplePoint ..
type ResourceUsageSamplePoint struct {
	CPU    int64 `json:"cpu"`
	Memory int64 `json:"mem"`
}

// EventLog is an event logger for a mariner component (i.e., engine or task)
type EventLog struct {
	sync.RWMutex
	Events []string `json:"events,omitempty"`
}

// update log (i.e., write to log file) each time there's an error, to capture point of failure
func (engine *K8sEngine) errorf(f string, v ...interface{}) error {
	err := engine.Log.Main.Event.errorf(f, v...)
	engine.writeLogToS3()
	return err
}

func (engine *K8sEngine) warnf(f string, v ...interface{}) {
	engine.Log.Main.Event.warnf(f, v...)
	engine.writeLogToS3()
}

func (engine *K8sEngine) infof(f string, v ...interface{}) {
	engine.Log.Main.Event.infof(f, v...)
	engine.writeLogToS3()
}

func (task *Task) errorf(f string, v ...interface{}) error {
	return task.Log.Event.errorf(f, v...)
}

func (task *Task) warnf(f string, v ...interface{}) {
	task.Log.Event.warnf(f, v...)
}

func (task *Task) infof(f string, v ...interface{}) {
	task.Log.Event.infof(f, v...)
}

// a record is "<timestamp> - <level> - <message>"
func (log *EventLog) write(level, message string) {
	log.Lock()
	defer log.Unlock()
	timestamp := ts()
	// timezone???
	record := fmt.Sprintf("%v - %v - %v", timestamp, level, message)
	log.Events = append(log.Events, record)
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

func (log *EventLog) errorf(f string, v ...interface{}) error {
	m := fmt.Sprintf(f, v...)
	return log.error(m)
}

func (log *EventLog) error(m string) error {
	log.write(errorLogLevel, m)
	return fmt.Errorf(m)
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
