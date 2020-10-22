package mariner

import (
	"fmt"
	"os"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	k8sCore "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	metricsBeta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsClient "k8s.io/metrics/pkg/client/clientset/versioned"
	metricsTyped "k8s.io/metrics/pkg/client/clientset/versioned/typed/metrics/v1beta1"
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
func dispatchWorkflowJob(workflowRequest *WorkflowRequest) (err error) {
	// get connection to cluster in order to dispatch jobs
	jobsClient, _, _, err := k8sClient(k8sJobAPI)
	if err != nil {
		return err
	}

	// create the workflow job spec (i.e., mariner-engine job spec)
	// `runID` is the jobName of the engine job
	jobSpec, err := workflowJob(workflowRequest)
	if err != nil {
		return fmt.Errorf("failed to create workflow job spec: %v", err)
	}

	// tell k8s to run this job
	workflowJob, err := jobsClient.Create(jobSpec)
	if err != nil {
		return fmt.Errorf("failed to create workflow job: %v", err)
	}

	// #logs
	fmt.Println("\tSuccessfully created workflow job.")
	fmt.Printf("\tNew job name: %v\n", workflowJob.Name)
	fmt.Printf("\tNew job UID: %v\n", workflowJob.GetUID())
	return nil
}

func (tool *Tool) sampleResourceUsage(podsClient corev1.PodInterface, label string) error {
	tool.Task.infof("begin sample resource usage")
	cpu, mem, err := tool.resourceUsage(podsClient, label)
	if err != nil {
		return tool.Task.errorf("failed to sample resource usage: %v", err)
	}
	p := ResourceUsageSamplePoint{
		CPU:    cpu,
		Memory: mem,
	}
	tool.Task.Log.Stats.ResourceUsage.Series.append(p)
	tool.Task.infof("end sample resource usage")
	return nil
}

func (tool *Tool) resourceUsage(podsClient corev1.PodInterface, label string) (cpu int64, mem int64, err error) {
	tool.Task.infof("begin get metrics from pod")
	podList, err := podsClient.List(metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return 0, 0, tool.Task.errorf("failed to fetch pod list: %v", err)
	}

	// nil value for a resource usage timepoint is (0, 0)
	// maybe there's a better way to represent this
	// I'd like to log every sampling interval
	// i.e., still log the event where resource usage was not available, as a (0,0) value
	switch l := len(podList.Items); l {
	case 0:
		return 0, 0, tool.Task.errorf("no pod found for task job")
	case 1:
		cpu, mem = containerResourceUsage(podList.Items[0], taskContainerName)
	default:
		// fixme - decide what to do here
		// currently expecting exactly one pod - though maybe it's possible there will be multiple pods,
		// if one pod is created but fails or errors, and the job controller creates a second pod
		// while the other one is error'ing or terminating
		// need to handle this case
		return 0, 0, tool.Task.errorf("found an unexpected number of pods associated with task job; njobs: %v ", l)
	}
	tool.Task.infof("end get metrics from pod; collected (cpu, mem) of (%v, %v)", cpu, mem)
	return cpu, mem, nil
}

// this job naming scheme makes the probability of having conflicting job names very low
func createJobName() string {
	return fmt.Sprintf("%v-%v", time.Now().Format("010206150405"), getRandString(5))
}

// routine for collecting (cpu, mem) usage over time per-task
func (engine *K8sEngine) collectResourceMetrics(tool *Tool) error {
	engine.infof("begin collect metrics for task: %v", tool.Task.Root.ID)
	// need to wait til pod exists
	// til metrics become available
	// as soon as they're available, begin logging them

	// NOTE: resource usage is a TIME SERIES - for now, we collect the whole thing
	// time points are every 30s (seems to be a k8s metrics monitoring default)

	// Q. how to handle task retries for metrics monitoring?
	// probably should keep the metrics for the failed attempt
	// and then separately track the metrics for each retry
	// this is to be handled when retry-logic is implemented
	// question is, will the job name be the same?
	// I believe so

	// keep sampling resource usage until task finishes
	_, podsClient, _, err := k8sClient(k8sPodAPI)
	if err != nil {
		tool.Task.Log.Event.warnf("%v", err)
		return err
	}
	label := fmt.Sprintf("job-name=%v", tool.Task.Log.JobName)

	engine.Lock()
	tool.Task.Log.Stats.ResourceUsage.init() // #race #ok
	engine.Unlock()

	done := false
	for !done {
		// collect (cpu, mem) sample point
		if err = tool.sampleResourceUsage(podsClient, label); err != nil {
			engine.Log.Main.Event.warnf("failed to sample resource usage for task: %v; error: %v", tool.Task.Root.ID, err)
		}

		// update logdb
		engine.Log.write()

		// wait out sampling period duration to next sample
		time.Sleep(metricsSamplingPeriod * time.Second)

		tool.Task.Lock()
		done = *tool.Task.Done // #race #ok
		tool.Task.Unlock()
	}

	engine.infof("end collect metrics for task: %v", tool.Task.Root.ID)

	return nil
}

func (engine *K8sEngine) dispatchTaskJob(tool *Tool) error {
	engine.infof("begin dispatch task job: %v", tool.Task.Root.ID)
	batchJob, err := engine.taskJob(tool)
	if err != nil {
		return engine.errorf("failed to load job spec for task: %v; error: %v", tool.Task.Root.ID, err)
	}
	jobsClient, _, _, err := k8sClient(k8sJobAPI)
	if err != nil {
		return engine.errorf("%v", err)
	}
	newJob, err := jobsClient.Create(batchJob)
	if err != nil {
		return engine.errorf("failed to create job for task: %v; error: %v", tool.Task.Root.ID, err)
	}
	engine.infof("created job with (name, id) (%v, %v) for task: %v", newJob.Name, newJob.GetUID(), tool.Task.Root.ID)

	// probably can make this nicer to look at
	tool.JobID = string(newJob.GetUID())

	tool.Task.Log.JobID = tool.JobID
	tool.Task.Log.JobName = tool.JobName
	engine.infof("end dispatch task job: %v", tool.Task.Root.ID)
	return nil
}

func metricsByPod() (*metricsBeta1.PodMetricsList, error) {
	_, _, podMetrics, err := k8sClient(k8sMetricsAPI)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}

	podMetricsList, err := podMetrics.List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return podMetricsList, nil
}

func containerMetrics(targetPod k8sCore.Pod, targetContainer string, pods *metricsBeta1.PodMetricsList) (*metricsBeta1.ContainerMetrics, error) {
	var containerMetrics metricsBeta1.ContainerMetrics
	var containerMetricsList []metricsBeta1.ContainerMetrics
	for _, i := range pods.Items {
		if i.Name == targetPod.Name {
			containerMetricsList = i.Containers
			for _, container := range containerMetricsList {
				if container.Name == targetContainer {
					containerMetrics = container
				}
			}
		}
	}

	if containerMetrics.Name == "" {
		return nil, fmt.Errorf("container %v of pod %v not found in list returned by metrics API", targetContainer, targetPod.Name)
	}

	return &containerMetrics, nil
}

// see: https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity.AsScale
func resourceUsage(container *metricsBeta1.ContainerMetrics) (cpu, mem int64) {

	// extract cpu usage - measured in "millicpu", where: 1000 millicpu == 1cpu
	// see: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/#meaning-of-cpu
	cpu = container.Usage.Cpu().MilliValue()

	// extract memory usage - measured in MB
	mem = container.Usage.Memory().ScaledValue(resource.Mega)

	return cpu, mem
}

// return (cpu, mem) for given (pod, container)
// if fail to collect, return (0,0)
// so (0,0) is the nil value
func containerResourceUsage(targetPod k8sCore.Pod, targetContainer string) (int64, int64) {
	pods, err := metricsByPod()
	if err != nil {
		// log
		return 0, 0
	}

	container, err := containerMetrics(targetPod, targetContainer, pods)
	if err != nil {
		// log
		return 0, 0
	}

	cpu, mem := resourceUsage(container)
	return cpu, mem
}

func k8sClient(k8sAPI string) (jobsClient batchtypev1.JobInterface, podsClient corev1.PodInterface, podMetricsClient metricsTyped.PodMetricsInterface, err error) {
	namespace := os.Getenv("GEN3_NAMESPACE")
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get k8s in-cluster config: %v", err)
	}
	if k8sAPI == k8sMetricsAPI {
		clientSet, err := metricsClient.NewForConfig(config)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get k8s metrics client: %v", err)
		}
		// podMetricsClient = clientSet.MetricsV1alpha1().PodMetricses(namespace)
		podMetricsClient = clientSet.MetricsV1beta1().PodMetricses(namespace)
		return nil, nil, podMetricsClient, nil
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get k8s clientset: %v", err)
	}
	switch k8sAPI {
	case k8sJobAPI:
		jobsClient = clientset.BatchV1().Jobs(namespace)
		return jobsClient, nil, nil, nil
	case k8sPodAPI:
		podsClient = clientset.CoreV1().Pods(namespace)
		return nil, podsClient, nil, nil
	}
	return
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
	// FIXME: don't hardcode ListOptions here like this
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
	jobsClient, _, _, err := k8sClient(k8sJobAPI)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}
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
// fixme #log - needs to be some logging mechanism for the server
// track events, errors, warnings
func deleteCompletedJobs() error {
	jobsClient, _, _, err := k8sClient(k8sJobAPI)
	if err != nil {
		// #log
		return err
	}
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
