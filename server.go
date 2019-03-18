package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/uc-cdis/gen3cwl/gen3cwl"
)

func makeRouter() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/", gen3cwl.HandleRoot).Methods("POST")
	router.HandleFunc("/_status", gen3cwl.HandleHealthcheck).Methods("GET")
	return router
}

func server() {

	httpLogger := log.New(os.Stdout, "", log.LstdFlags)
	httpLogger.Fatal(http.ListenAndServe("localhost:8000", makeRouter()))
}
