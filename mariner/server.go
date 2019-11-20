package mariner

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	batchv1 "k8s.io/api/batch/v1"
	batchtypev1 "k8s.io/client-go/kubernetes/typed/batch/v1"

	"github.com/uc-cdis/go-authutils/authutils"
)

// this file contains code for setting up the mariner-server
// and registering handler functions for the various WES API endpoints
// WES spec: https://github.com/ga4gh/workflow-execution-service-schemas/blob/master/openapi/workflow_execution_service.swagger.yaml
// NOTE: server is modeled after arborist

type WorkflowRequest struct {
	Workflow json.RawMessage   `json:"workflow"`
	Input    json.RawMessage   `json:"input"`
	UserID   string            `json:"user"`
	Tags     map[string]string `json:"tags,omitempty"` // optional set of key:val pairs provided by user to annotate workflow run - NOTE: val is a string
	Manifest Manifest          `json:"manifest"`
	JobName  string            `json:"jobName,omitempty"` // populated internally by server
}

type Manifest []ManifestEntry

type ManifestEntry struct {
	GUID string `json:"object_id"`
}

type JWTDecoder interface {
	Decode(string) (*map[string]interface{}, error)
}

type Server struct {
	jwtApp JWTDecoder
	logger *LogHandler
}

// see Arborist's logging.go
// need to handle server logging
type LogHandler struct {
	logger *log.Logger
}

type AuthHTTPRequest struct {
	URL         string
	ContentType string
	Body        io.Reader
}

type RequestJSON struct {
	User    *UserJSON    `json:"user"`
	Request *AuthRequest `json:"request"`
}

type AuthRequest struct {
	Resource string      `json:"resource"`
	Action   *AuthAction `json:"action"`
}

type AuthAction struct {
	Service string `json:"service"`
	Method  string `json:"method"`
}

type UserJSON struct {
	Token string `json:"token"`
}

type RunLogJSON struct {
	Log *MainLog `json:"log"`
}

type StatusJSON struct {
	Status string `json:"status"`
}

type CancelRunJSON struct {
	RunID  string `json:"runID"`
	Result string `json:"result"` // success or failed
}

type ListRunsJSON struct {
	RunIDs []string `json:"runIDs"`
}

type RunIDJSON struct {
	RunID string `json:"runID"`
}

type ArboristResponse struct {
	Auth bool `json:"auth"`
}

// RunServer inits the mariner server
func RunServer() {
	go deleteCompletedJobs()
	runServer()
}

// runServer sets up and runs the mariner server
func runServer() {
	jwkEndpointEnv := os.Getenv("JWKS_ENDPOINT")
	port := flag.Uint("port", 80, "port on which to expose the API")
	jwkEndpoint := flag.String(
		"jwks",
		jwkEndpointEnv,
		"endpoint from which the application can fetch a JWKS",
	)
	logFlags := log.Ldate | log.Ltime
	logger := log.New(os.Stdout, "", logFlags)
	jwtApp := authutils.NewJWTApplication(*jwkEndpoint)
	server := server().withLogger(logger).withJWTApp(jwtApp)
	router := server.makeRouter(os.Stdout)
	addr := fmt.Sprintf(":%d", *port)
	httpLogger := log.New(os.Stdout, "", log.LstdFlags)
	httpServer := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		ErrorLog:     httpLogger,
		Handler:      router,
	}
	httpLogger.Println(fmt.Sprintf("mariner serving at %s", httpServer.Addr))
	httpLogger.Fatal(httpServer.ListenAndServe())
}

func (server *Server) withJWTApp(jwtApp JWTDecoder) *Server {
	server.jwtApp = jwtApp
	return server
}

// TODO - see logging in mariner - implement server logging for mariner
func (server *Server) withLogger(logger *log.Logger) *Server {
	server.logger = &LogHandler{logger: logger}
	return server
}

func server() (server *Server) {
	return &Server{}
}

// first just getting the endpoints to work, then will make nice and WES-ish
func (server *Server) makeRouter(out io.Writer) http.Handler {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/runs", server.handleRunsPOST).Methods("POST")
	router.HandleFunc("/runs", server.handleRunsGET).Methods("GET")
	router.HandleFunc("/runs/{runID}", server.handleRunLogGET).Methods("GET")
	router.HandleFunc("/runs/{runID}/status", server.handleRunStatusGET).Methods("GET")
	router.HandleFunc("/runs/{runID}/cancel", server.handleCancelRunPOST).Methods("POST")
	router.HandleFunc("/_status", server.handleHealthCheck).Methods("GET") // TO CHECK

	// router.NotFoundHandler = http.HandlerFunc(handleNotFound) // TODO

	router.Use(server.handleAuth)        // use auth middleware function - right now access to mariner API is all-or-nothing
	router.Use(server.setResponseHeader) // set "Content-Type: application/json" header - every endpoint returns JSON

	// remove trailing slashes sent in URLs
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		router.ServeHTTP(w, r)
	})

	// unsure if this is the/a logging solution that we want
	// but it seems to be a standard, and arborist uses it
	// keeping it for now - TODO: design and implement logging for server
	// see: https://godoc.org/github.com/gorilla/handlers#CombinedLoggingHandler
	return handlers.CombinedLoggingHandler(out, handler)
}

//// handlers ////

// '/runs/{runID}' - GET
func (server *Server) handleRunLogGET(w http.ResponseWriter, r *http.Request) {
	userID, runID := server.uniqueKey(r)
	j, err := (&RunLogJSON{}).fetchLog(userID, runID)
	if err != nil {
		fmt.Println("error fetching log: ", err)
		// handle
	}
	writeJSON(w, j)
}

func (j *RunLogJSON) fetchLog(userID, runID string) (*RunLogJSON, error) {
	runLog, err := fetchMainLog(userID, runID)
	if err != nil {
		return nil, err
	}
	j.Log = runLog
	return j, nil
}

// '/runs/{runID}/status' - GET
func (server *Server) handleRunStatusGET(w http.ResponseWriter, r *http.Request) {
	userID, runID := server.uniqueKey(r)
	j, err := (&StatusJSON{}).fetchStatus(userID, runID)
	if err != nil {
		fmt.Println("error fetching status: ", err)
		// handle
	}
	writeJSON(w, j)
}

func (j *StatusJSON) fetchStatus(userID, runID string) (*StatusJSON, error) {
	runLog, err := fetchMainLog(userID, runID)
	if err != nil {
		return nil, err
	}
	j.Status = runLog.Main.Status
	return j, nil
}

// '/runs/{runID}/cancel' - POST
func (server *Server) handleCancelRunPOST(w http.ResponseWriter, r *http.Request) {
	userID, runID := server.uniqueKey(r)
	j, err := (&CancelRunJSON{}).cancelRun(userID, runID)
	if err != nil {
		fmt.Println("error cancelling run: ", err)
	}
	writeJSON(w, j)
}

// FIXME - try to kill as many processes as possible
// i.e., don't return at each possible error - run the whole thing (attempt everything)
// and return errors at end
// TODO - LOG this event
func (j *CancelRunJSON) cancelRun(userID, runID string) (*CancelRunJSON, error) {
	j.RunID = runID
	j.Result = failed
	runLog, err := fetchMainLog(userID, runID)
	if err != nil {
		return j, err
	}
	jc := jobClient()
	engineJob, err := jobByID(jc, runLog.Main.JobID)
	if err != nil {
		return j, err
	}

	// first kill engine job
	fmt.Println("deleting engine job..")
	err = deleteJobs([]batchv1.Job{*engineJob}, running, jc)
	if err != nil {
		return j, err
	}

	// then wait til engine job is killed, and kill all associated task jobs
	go func(runLog *MainLog, jc batchtypev1.JobInterface) {
		fmt.Println("sleeping out grace period..")
		time.Sleep(150 * time.Second)

		fmt.Println("collecting tasks..")
		taskJobs := []batchv1.Job{}
		for taskID, task := range runLog.ByProcess {
			fmt.Println("handling task ", taskID)
			if task.JobID != "" {
				fmt.Println("nonempty jobID: ", task.JobName)
				job, err := jobByID(jc, task.JobID)
				if err != nil {
					fmt.Println("failed to fetch job with ID ", task.JobID)
				}
				fmt.Println("collected this job: ", task.JobName)
				taskJobs = append(taskJobs, *job)
			}
		}
		fmt.Println("deleting task jobs..")
		err = deleteJobs(taskJobs, running, jc)
		if err != nil {
			fmt.Println("error deleting task jobs: ", err)
		}
		fmt.Println("successfully deleted task jobs")
	}(runLog, jc)

	j.Result = success
	return j, nil
}

// '/runs' - GET
func (server *Server) handleRunsGET(w http.ResponseWriter, r *http.Request) {
	userID := server.userID(r)
	j, err := (&ListRunsJSON{}).fetchRuns(userID)
	if err != nil {
		fmt.Println("error fetching runs: ", err)
	}
	writeJSON(w, j)
}

func (j *ListRunsJSON) fetchRuns(userID string) (*ListRunsJSON, error) {
	runIDs, err := listRuns(userID)
	if err != nil {
		return nil, err
	}
	j.RunIDs = runIDs
	return j, nil
}

// `/runs` - POST
func (server *Server) handleRunsPOST(w http.ResponseWriter, r *http.Request) {
	workflowRequest := unmarshalBody(r, &WorkflowRequest{}).(*WorkflowRequest)
	workflowRequest.UserID = server.userID(r)
	runID, err := dispatchWorkflowJob(workflowRequest)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	j := &RunIDJSON{RunID: runID}
	writeJSON(w, j)
}

//// middleware ////

// all endpoints return JSON, so just set that response header here
func (server *Server) setResponseHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// auth middleware - processes every request, checks auth with arborist
// if arborist says 'okay', then process the request
// if arborist says 'not okay', then http error 'not authorized'
func (server *Server) handleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if server.authZ(r) {
			// fmt.Println("user has access") // log
			next.ServeHTTP(w, r)
			return
		}
		// fmt.Println("user does NOT have access") // log
		http.Error(w, "user not authorized to access this resource", 403)
	})
}

// polish this
func authHTTPRequest(r *http.Request) (*AuthHTTPRequest, error) {
	token := r.Header.Get(authHeader)
	if token == "" {
		return nil, fmt.Errorf("no token in Authorization header")
	}
	user := &UserJSON{
		Token: token,
	}
	authRequest := &AuthRequest{
		Resource: "/mariner",
	}
	authAction := &AuthAction{
		Service: "mariner",
		Method:  "access",
	}
	authRequest.Action = authAction
	requestJSON := &RequestJSON{
		User:    user,
		Request: authRequest,
	}
	b, err := json.Marshal(requestJSON)
	if err != nil {
		fmt.Println("error marhsaling authRequest to json: ", err)
	}
	authHTTPRequest := &AuthHTTPRequest{
		URL:         "http://arborist-service/auth/request",
		ContentType: "application/json",
		Body:        bytes.NewBuffer(b),
	}
	return authHTTPRequest, nil
}

func (server *Server) authZ(r *http.Request) bool {
	authHTTPRequest, err := authHTTPRequest(r)
	if err != nil {
		fmt.Println("error building auth request: ", err)
		return false
	}
	resp, err := http.Post(
		authHTTPRequest.URL,
		authHTTPRequest.ContentType,
		authHTTPRequest.Body,
	)
	if err != nil {
		// insert better error and logging handling here
		fmt.Println("error asking arborist: ", err)
		return false
	}
	authResponse := &ArboristResponse{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading arborist response body: ", err)
		return false
	}
	resp.Body.Close()
	err = json.Unmarshal(b, authResponse)
	if err != nil {
		fmt.Println("error unmarshalling arborist response to struct: ", err)
		return false
	}
	return authResponse.Auth
}

// HandleHealthcheck registers root endpoint
// FIXME
func (server *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL)
	return
}

//// Server utility functions ////

func writeJSON(w http.ResponseWriter, j interface{}) {
	e := json.NewEncoder(w)
	e.SetIndent("", "    ")
	e.Encode(j)
}

// a run's unique key is the pair (userID, runID)
func (server *Server) uniqueKey(r *http.Request) (userID, runID string) {
	runID = mux.Vars(r)["runID"]
	userID = server.userID(r)
	return userID, runID
}

// unmarshal the request body to the given go struct
func unmarshalBody(r *http.Request, v interface{}) interface{} {
	b := body(r)
	err := json.Unmarshal(b, v)
	if err != nil {
		fmt.Println("error unmarshalling: ", err)
	}
	return v
}

func body(r *http.Request) []byte {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("error reading body: ", err)
	}
	r.Body.Close()
	r.Body = ioutil.NopCloser(bytes.NewBuffer(b))
	return b
}
