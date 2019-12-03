package mariner

import (
	"fmt"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	// metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// this file contains code for interacting with k8s cluster via k8s api
// e.g., get cluster config, handle runtime requirements, create job spec, execute job, get job info/status
// NOTE: clean up the code - move all the config/spec-related things into a separate file

// JobInfo - k8s job information
type JobInfo struct {
	UID    string `json:"uid"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// DispatchWorkflowJob runs a workflow provided in mariner api request
// TODO - decide on an approach to error handling, and apply it uniformly
func dispatchWorkflowJob(workflowRequest *WorkflowRequest) (runID string, err error) {
	// get connection to cluster in order to dispatch jobs
	jobsClient, _ := k8sClient(k8sJobAPI)

	// create the workflow job spec (i.e., mariner-engine job spec)
	jobSpec, runID, err := workflowJob(workflowRequest)
	if err != nil {
		return "", fmt.Errorf("failed to create workflow job spec: %v", err)
	}

	// tell k8s to run this job
	workflowJob, err := jobsClient.Create(jobSpec)
	if err != nil {
		return "", fmt.Errorf("failed to create workflow job: %v", err)
	}

	// logs
	fmt.Println("\tSuccessfully created workflow job.")
	fmt.Printf("\tNew job name: %v\n", workflowJob.Name)
	fmt.Printf("\tNew job UID: %v\n", workflowJob.GetUID())
	return runID, nil
}

// dev'ing
func (tool *Tool) collectResourceUsage() {
	// podsClient := core.
	// label := fmt.Sprintf("job-name=%v", tool.Task.Log.JobName)

	// need to wait til pod exists
	// til metrics become available
	// as soon as they're available, log them as a time series

	// Q. does resource usage vary over time, or is it sufficient to collect them once?
	// A. resource usage is a TIME SERIES - for now, collect the whole thing
	// -- maybe in the end will only want min/max/mean etc.

	// HERE
	//	_, podsClient := k8sClient(k8sPodAPI)
	// ...
	/*
		var cpu, mem *ResourceStat = &tool.Task.Log.Stats.CPU, &tool.Task.Log.Stats.Memory
		for cpu.Actual == 0 && mem.Actual == 0 {
		}
	*/

	return
}

func (engine K8sEngine) dispatchTaskJob(tool *Tool) error {
	fmt.Println("\tCreating k8s job spec..")
	batchJob, nil := engine.taskJob(tool)
	jobsClient, _ := k8sClient(k8sJobAPI)
	fmt.Println("\tRunning k8s job..")
	newJob, err := jobsClient.Create(batchJob)
	if err != nil {
		fmt.Printf("\tError creating job: %v\n", err)
		return err
	}
	fmt.Println("\tSuccessfully created job.")
	fmt.Printf("\tNew job name: %v\n", newJob.Name)
	fmt.Printf("\tNew job UID: %v\n", newJob.GetUID())

	// probably can make this nicer to look at
	tool.JobID = string(newJob.GetUID())
	tool.JobName = newJob.Name

	tool.Task.Log.JobID = tool.JobID
	tool.Task.Log.JobName = tool.JobName
	return nil
}

func k8sClient(k8sAPI string) (jobsClient batchtypev1.JobInterface, podsClient v1.PodInterface) {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	namespace := os.Getenv("GEN3_NAMESPACE")
	switch k8sAPI {
	case k8sJobAPI:
		jobsClient = clientset.BatchV1().Jobs(namespace)
	case k8sPodAPI:
		podsClient = clientset.CoreV1().Pods(namespace)
	}
	return jobsClient, podsClient
}

func jobByID(jc batchtypev1.JobInterface, jobID string) (*batchv1.Job, error) {
	jobs, err := listMarinerJobs(jc)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if jobID == string(job.GetUID()) {
			return &job, nil
		}
	}
	return nil, fmt.Errorf("job with jobid %s not found", jobID)
}

// trade engine jobName for engine jobID
func engineJobID(jc batchtypev1.JobInterface, jobName string) string {
	// FIXME - create "interface" for fetching particular sets of jobs
	// i.e., listTaskJobs, listEngineJobs, etc.
	// don't hardcode ListOptions here like this
	engines, err := jc.List(metav1.ListOptions{LabelSelector: "app=mariner-engine"})
	if err != nil {
		// log
		fmt.Println("error fetching engine job list: ", err)
		return ""
	}
	for _, job := range engines.Items {
		if jobName == string(job.GetName()) {
			return string(job.GetUID())
		}
	}
	fmt.Printf("error: job with jobName %s not found", jobName)
	return ""
}

func jobStatusByID(jobID string) (*JobInfo, error) {
	jobsClient, _ := k8sClient(k8sJobAPI)
	job, err := jobByID(jobsClient, jobID)
	if err != nil {
		return nil, err
	}
	i := JobInfo{}
	i.Name = job.Name
	i.UID = string(job.GetUID())
	i.Status = jobStatusToString(&job.Status)
	return &i, nil
}

// see: https://kubernetes.io/docs/api-reference/batch/v1/definitions/#_v1_jobstatus
func jobStatusToString(status *batchv1.JobStatus) string {
	if status == nil {
		return unknown
	}
	if status.Succeeded >= 1 {
		return completed
	}
	if status.Failed >= 1 {
		return failed
	}
	if status.Active >= 1 {
		return running
	}
	return unknown
}

// background process that collects status of mariner jobs
// jobs with status COMPLETED are deleted
// ---> since all logs/other information are collected immmediately when the job finishes
func deleteCompletedJobs() {
	jobsClient, _ := k8sClient(k8sJobAPI)
	for {
		jobs, err := listMarinerJobs(jobsClient)
		if err != nil {
			fmt.Println("Jobs monitoring error: ", err)
			time.Sleep(30 * time.Second)
			continue
		}
		time.Sleep(30 * time.Second)
		deleteJobs(jobs, completed, jobsClient)
		time.Sleep(30 * time.Second)
	}
}

// 'condition' is a jobStatus, as in a value returned by jobStatusToString()
// NOTE: probably should pass a list of conditions, not a single string
func deleteJobs(jobs []batchv1.Job, condition string, jobsClient batchtypev1.JobInterface) error {
	if condition == running {
		fmt.Println(" --- run cancellation - in deleteJobs() ---")
	}
	deleteOption := metav1.NewDeleteOptions(120) // how long (seconds) should the grace period be?
	var deletionPropagation metav1.DeletionPropagation = "Background"
	deleteOption.PropagationPolicy = &deletionPropagation
	for _, job := range jobs {
		k8sJob, err := jobStatusByID(string(job.GetUID()))
		if err != nil {
			fmt.Println("Can't get job status by UID: ", job.Name, err)
		} else {
			if k8sJob.Status == condition {
				fmt.Printf("Deleting job %v under condition %v\n", job.Name, condition)
				err = jobsClient.Delete(job.Name, deleteOption)
				if err != nil {
					fmt.Println("Error deleting job : ", job.Name, err)
					return err
				}
			}
		}
	}
	return nil
}

func listMarinerJobs(jobsClient batchtypev1.JobInterface) ([]batchv1.Job, error) {
	jobs := []batchv1.Job{}
	tasks, err := jobsClient.List(metav1.ListOptions{LabelSelector: "app=mariner-task"})
	if err != nil {
		return nil, err
	}
	engines, err := jobsClient.List(metav1.ListOptions{LabelSelector: "app=mariner-engine"})
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, tasks.Items...)
	jobs = append(jobs, engines.Items...)
	return jobs, nil
}
