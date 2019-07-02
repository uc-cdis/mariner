package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/uc-cdis/gen3cwl/mariner"
)

func makeRouter() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/", mariner.HandleRoot).Methods("POST")
	router.HandleFunc("/_status", mariner.HandleHealthcheck).Methods("GET")
	return router
}

func server() {

	httpLogger := log.New(os.Stdout, "", log.LstdFlags)
	httpLogger.Fatal(http.ListenAndServe("localhost:8000", makeRouter()))
}
