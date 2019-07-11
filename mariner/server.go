package mariner

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func makeRouter() *mux.Router {
	router := mux.NewRouter()

	router.HandleFunc("/run", HandleRoot).Methods("POST")
	router.HandleFunc("/_status", HandleHealthcheck).Methods("GET")
	return router
}

func Server() {
	httpLogger := log.New(os.Stdout, "", log.LstdFlags)
	httpLogger.Fatal(http.ListenAndServe(":8000", makeRouter()))
}
