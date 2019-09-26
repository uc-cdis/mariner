package mariner

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

// this file contains code for setting up the mariner-server
// and registering handler functions for the various api endpoints
// notice: the api needs to be extended

func makeRouter() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/run", RunHandler).Methods("POST")
	router.HandleFunc("/_status", HandleHealthcheck).Methods("GET")
	return router
}

func Server() {
	httpLogger := log.New(os.Stdout, "", log.LstdFlags)
	httpLogger.Fatal(http.ListenAndServe(":80", makeRouter()))
}
