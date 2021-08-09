package mariner

import (
	"database/sql"
	"io/ioutil"

	"fmt"

	log "github.com/sirupsen/logrus"

	"errors"

	"os"

	_ "github.com/lib/pq"

	"encoding/json"
)

type DBCredentials struct {
	Host         string `json:"db_host"`
	User         string `json:"db_username"`
	Password     string `json:"db_password"`
	DatabaseName string `json:"db_database"`
}

const (
	dbcredentialpath = "/var/www/mariner/dbcreds.json"
)

type PSQLDataBase struct {
	DBBase
	DBConnection *sql.DB
}

func (db *PSQLDataBase) getCredentials() {
	credFile, err := os.Open(dbcredentialpath)
	if err != nil {
		log.Error("Could not open database credential file ", err)
	}
	defer credFile.Close()

	byteValue, _ := ioutil.ReadAll(credFile)

	var credential DBCredentials
	err = json.Unmarshal(byteValue, &credential)
	if err != nil {
		log.Error("Marshling credential file failed ", err)
	}

	db.DBHost = credential.Host
	db.DBUser = credential.User
	db.DBPassword = credential.Password //pragma: allowlist secret
	db.DBName = credential.DatabaseName
}

func (db *PSQLDataBase) Initalize() {
	db.DBType = PostgresDB
	db.getCredentials()
}

func (db *PSQLDataBase) Connect() (string, error) {
	psqlInfo := fmt.Sprintf("host=%s user=%s "+
		"password=%s dbname=%s sslmode=disable", //pragma: allowlist secret
		db.DBHost, db.DBUser, db.DBPassword, db.DBName)

	dbConnection, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		log.Error("connection to database %s failed", db.DBName)
		dbConnection.Close()
		errorString := fmt.Sprintf("connection to database %s failed", db.DBName)
		return "", errors.New(errorString)
	}

	err = dbConnection.Ping()
	if err != nil {
		log.Error("could not ping database %s after connection", db.DBName)
		dbConnection.Close()
		errorString := fmt.Sprintf("could not ping database %s after connection", db.DBName)
		return "", errors.New(errorString)
	}

	log.Info("Database connection to %s established", db.DBName)

	db.DBConnection = dbConnection

	sucessString := fmt.Sprintf("connection to %s established", db.DBName)
	return sucessString, nil
}

func (db *PSQLDataBase) get(fields []string, tablename string, query string) []string {
	//sqlString := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(fields, ","), tablename, query)
	rval := []string{"temp"}
	return rval
}
