package mariner

import (
	"database/sql"
	"io/ioutil"
	"strings"
	"time"

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

	db.DBConnection = dbConnection

	sucessString := fmt.Sprintf("connection to %s established", db.DBName)
	log.Info(sucessString)
	return sucessString, nil
}

func makeUsrTableValues(fields []string, values []interface{}) {
	for index, field := range fields {
		switch field {
		case "id":
			values[index] = new(int)
		case "name", "email":
			values[index] = new(string)
		case "created_at":
			values[index] = new(time.Time)
		default:
			warningString := fmt.Sprintf("the field %s is defaulted to be unmarshalled as a string, please set it to correct field type and not default to a value", field)
			log.Warn(warningString)
			values[index] = new(string)
		}
	}
}

func makeTaskTableValues(fields []string, values []interface{}) {
	for index, field := range fields {
		switch field {
		case "task_id", "wf_id", "attemp", "return_code":
			values[index] = new(int)
		case "name", "hash", "stats", "output", "status", "error", "wf_status", "input":
			values[index] = new(string)
		case "created_at", "updated_at":
			values[index] = new(time.Time)
		default:
			warningString := fmt.Sprintf("the field %s is defaulted to be unmarshalled as a string, please set it to correct field type and not default to a value", field)
			log.Warn(warningString)
			values[index] = new(string)
		}
	}
}

func makeWorkFlowTableValues(fields []string, values []interface{}) {
	for index, field := range fields {
		switch field {
		case "task_id", "wf_id", "last_task_completed":
			values[index] = new(int)
		case "definition", "hash", "stats", "output", "status", "input", "metadata":
			values[index] = new(string)
		case "started_at", "ended_at", "created_at", "updated_at":
			values[index] = new(time.Time)
		default:
			warningString := fmt.Sprintf("the field %s is defaulted to be unmarshalled as a string, please set it to correct field type and not default to a value", field)
			log.Warn(warningString)
			values[index] = new(string)
		}
	}
}

func makeDynamicValues(fields []string, tablename string) []interface{} {
	values := make([]interface{}, len(fields))
	switch tablename {
	case usrTable:
		makeUsrTableValues(fields, values)
	case taskTable:
		makeTaskTableValues(fields, values)
	case workflowTable:
		makeWorkFlowTableValues(fields, values)
	default:
		errorString := fmt.Sprintf("the table %s does not have a seralization format in code, please add seralization format if this is a new table and check if table name is correctly spelled", tablename)
		log.Error(errorString)
	}
	return values
}

func (db *PSQLDataBase) Get(tablename string, fields []string, filter string) ([][]string, error) {
	sqlString := fmt.Sprintf("SELECT %s FROM %s", strings.Join(fields, ","), tablename)
	if len(filter) > 0 {
		sqlString = fmt.Sprintf("%s WHERE %s", sqlString, filter)
	}

	rows, err := db.DBConnection.Query(sqlString)
	if err != nil {
		errorString := fmt.Sprintf("database connection failed with error %s", err.Error())
		log.Error(errorString)
		return [][]string{{"select failed"}}, errors.New(errorString)
	}

	log.Info("this is the table ", tablename)

	values := makeDynamicValues(fields, tablename)
	var fieldValues [][]string
	for rows.Next() {
		err = rows.Scan(values...)
		if err != nil {
			log.Error(err)
		}

		var tempFieldValues []string
		for _, val := range values {
			b, _ := json.Marshal(val)
			tempFieldValues = append(tempFieldValues, string(b))
		}

		fieldValues = append(fieldValues, tempFieldValues)
	}

	return fieldValues, nil
}

func (db *PSQLDataBase) sqlExecutionHelper(operation string, sqlString string) (string, error) {
	res, err := db.DBConnection.Exec(sqlString)
	if err != nil {
		errorString := fmt.Sprintf("%s failed executing sql string with error %s", operation, err.Error())
		log.Error(errorString)
		return "delete failed", errors.New(errorString)
	}

	count, err := res.RowsAffected()
	if err != nil {
		errorString := fmt.Sprintf("%s failed reading rows affected with error %s", operation, err.Error())
		log.Error(errorString)
		return "delete failed", errors.New(errorString)
	}

	sucessString := fmt.Sprintf("%s succeeded and %d rows were affected", operation, count)
	return sucessString, nil
}

func (db *PSQLDataBase) Update(tablename string, fields []string, updatedFieldValues []string, filter string) (string, error) {
	if len(fields) != len(updatedFieldValues) {
		errorString := "the number of fields to update and updated values do not match up, please double check"
		log.Error(errorString)
		return "update failed", errors.New(errorString)
	}

	var fieldAndValuePair []string
	for index := range fields {
		pairString := fmt.Sprintf("%s=%s", fields[index], updatedFieldValues[index])
		fieldAndValuePair = append(fieldAndValuePair, pairString)
	}

	sqlString := fmt.Sprintf("UPDATE %s SET %s WHERE %s", tablename, strings.Join(fieldAndValuePair, ","), filter)
	return db.sqlExecutionHelper("update", sqlString)
}

func (db *PSQLDataBase) Delete(tablename string, filter string) (string, error) {
	sqlString := fmt.Sprintf("DELETE FROM %s WHERE %s", tablename, filter)
	return db.sqlExecutionHelper("delete", sqlString)
}

func (db *PSQLDataBase) Insert(tablename string, fields []string, fieldValues []string) (string, error) {
	if len(fields) != len(fieldValues) {
		errorString := "the number of fields and values do not match up, please double check"
		log.Error(errorString)
		return "update failed", errors.New(errorString)
	}

	sqlString := fmt.Sprintf("INSERT INTO %s (%s) VALUES(%s)", tablename, strings.Join(fields, ","), strings.Join(fieldValues, ","))
	return db.sqlExecutionHelper("insert", sqlString)
}
