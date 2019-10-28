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
	"time"

	"github.com/gorilla/mux"

	"github.com/uc-cdis/go-authutils/authutils"
)

// this file contains code for setting up the mariner-server
// and registering handler functions for the various api endpoints
// NOTE: server is modeled after arborist

// WorkflowRequest holds the unmarshalled body of the POST request
// where
// Workflow is the packed CWL workflow JSON (i.e., all the CWL files packed into a JSON - ii.ee., the result of cwltool --pack)
// Input is the JSON specifying values for the input parameters to the workflow (refer to files using GUIDs)
// ID is the userID
// HERE - TODO - eventually replace "ID" field with "token"
// ---> then need to retrieve user ID by trade token with Fence
type WorkflowRequest struct {
	Workflow json.RawMessage `json:"workflow"`
	Input    json.RawMessage `json:"input"`
	UserID   string          `json:"id"`
	Token    string          `json:"token"` // flow not yet implemented, but field defined here
	Manifest Manifest        `json:"manifest"`
}

// HERE - TODO - move to config.go
type Manifest []ManifestEntry
type ManifestEntry struct {
	GUID string `json:"object_id"`
}

type JWTDecoder interface {
	Decode(string) (*map[string]interface{}, error) // not sure why this is a pointer to a map? map is already passed by reference
}

type Server struct {
	jwtApp JWTDecoder
	logger *LogHandler
}

// move to log.go
// see Arborist's logging.go
// need to integrate or ow handle server logging vs. workflow logging
type LogHandler struct {
	logger *log.Logger
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

func (server *Server) makeRouter(out io.Writer) http.Handler {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/run", server.runHandler).Methods("POST")
	router.HandleFunc("/_status", server.handleHealthcheck).Methods("GET")
	return router
}

func server() (server *Server) {
	return &Server{}
}

// Server runs the mariner server that listens for API calls
func RunServer() {
	jwkEndpointEnv := os.Getenv("JWKS_ENDPOINT") // TODO - add this environment variable to mariner-deployment pod spec - see arborist deployment

	// Parse flags:
	//     - port (to serve on)
	//     - jwks (endpoint to get keys for JWT validation)
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

// RunHandler handles `/run` endpoint
// handles a POST request to run a workflow by dispatching the workflow job
// see "../testdata/request_body.json" for an example of a valid request body
// also see above description of the fields of the WorkflowRequest struct
// since those are the same fields as the request body
// NOTE: come up with uniform, sensible names for handler functions
func (server *Server) runHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("handling root mariner request..")
	fmt.Println(r.URL)

	workflowRequest := workflowRequest(r)

	workflowRequest.UserID = server.userID(workflowRequest.Token)

	// dispatch job to k8s to run workflow
	// -> `mariner run $S3PREFIX`, where
	// S3PREFIX is the working directory for this workflow in the workflow bucket
	fmt.Printf("running workflow for user %v\n", workflowRequest.UserID)
	printJSON(workflowRequest)
	err := dispatchWorkflowJob(workflowRequest)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
}

func workflowRequest(r *http.Request) *WorkflowRequest {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("error reading request body: ", err)
	}
	// probably this variable should be a pointer, not the val itself
	workflowRequest := &WorkflowRequest{}
	err = json.Unmarshal(body, workflowRequest)
	if err != nil {
		fmt.Printf("fail to parse json %v\n", err)
	}
	return workflowRequest
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

// auth middleware - processes every request, checks auth with arborist
// if arborist says 'okay', then process the request
// if arborist says 'not okay', then http error 'not authorized'
// need to have router.Use(authRequest) or something like that - need to add it to router
func (server *Server) handleAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if server.authZ(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Error(w, "user not authorized to access this resource", 403)
	})
}

// polish this
func authHTTPRequest(r *http.Request) *AuthHTTPRequest {
	workflowRequest := workflowRequest(r)
	user := &UserJSON{
		Token: workflowRequest.Token,
	}
	// double check these things
	authRequest := &AuthRequest{
		Resource: "mariner",
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
		URL:         "arborist-service/auth/request",
		ContentType: "application/json",
		Body:        bytes.NewBuffer(b),
	}
	return authHTTPRequest
}

func (server *Server) authZ(r *http.Request) bool {
	authHTTPRequest := authHTTPRequest(r)
	resp, err := http.Post(
		authHTTPRequest.URL,
		authHTTPRequest.ContentType,
		authHTTPRequest.Body,
	)
	if err != nil {
		// insert better error and logging handling here
		fmt.Println("error asking arborist: ", err)
	}
	authResponse := &ArboristResponse{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading arborist response body: ", err)
	}
	err = json.Unmarshal(b, authResponse)
	if err != nil {
		fmt.Println("error unmarshalling arborist response to struct: ", err)
	}
	return authResponse.Auth
}

type ArboristResponse struct {
	Auth bool `json:"auth"`
}

// HandleHealthcheck registers root endpoint
func (server *Server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL)
	return
}
