package database

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	logrus "github.com/sirupsen/logrus"
)

type DBCredentials struct {
	Host         string `json:"db_host"`
	User         string `json:"db_username"`
	Password     string `json:"db_password"`
	DatabaseName string `json:"db_database"`
}

type PSQLDao struct {
	Host         string
	User         string
	Password     string
	DBName       string
	DBConnection *sqlx.DB
}

func connect(psqlDao *PSQLDao) (string, error) {
	psqlInfo := fmt.Sprintf("host=%s user=%s "+
		"password=%s dbname=%s sslmode=disable", //pragma: allowlist secret
		psqlDao.Host, psqlDao.User, psqlDao.Password, psqlDao.DBName)

	dbConnection, err := sqlx.Open("postgres", psqlInfo)

	if err != nil {
		logrus.Errorf("connection to database %s failed", psqlDao.DBName)
		dbConnection.Close()
		err := fmt.Errorf("connection to database %s failed", psqlDao.DBName)
		return "", err
	}

	err = dbConnection.Ping()
	if err != nil {
		logrus.Errorf("could not ping database %s after connection", psqlDao.DBName)
		dbConnection.Close()
		err := fmt.Errorf("could not ping database %s after connection", psqlDao.DBName)
		return "", err
	}

	psqlDao.DBConnection = dbConnection

	sucessString := fmt.Sprintf("connection to %s established", psqlDao.DBName)
	logrus.Info(sucessString)
	return sucessString, nil
}

func mustGetCredentials(psqlDao *PSQLDao) {
	credFile, err := os.Open(dbcredentialpath)
	if err != nil {
		logrus.Fatal("Could not open database credential file ", err)
	}
	defer credFile.Close()

	byteValue, _ := ioutil.ReadAll(credFile)

	var credential DBCredentials
	err = json.Unmarshal(byteValue, &credential)
	if err != nil {
		logrus.Fatal("Marshling credential file failed ", err)
	}

	psqlDao.Host = credential.Host
	psqlDao.User = credential.User
	psqlDao.Password = credential.Password //pragma: allowlist secret
	psqlDao.DBName = credential.DatabaseName
}

func NewPSQLDao() *PSQLDao {
	var newDao PSQLDao
	mustGetCredentials(&newDao)
	_, err := connect(&newDao)
	if err != nil {
		return nil
	}
	return &newDao
}

func (psqlDao *PSQLDao) GetUserById(id int64) (*User, error) {
	user := User{}
	query := fmt.Sprintf("SELECT * FROM usr WHERE id=%d", id)
	err := psqlDao.DBConnection.Get(&user, query)

	if err != nil {
		logrus.Errorf("Could not retrieve user with id %d, failed with error %s", id, err)
		return nil, fmt.Errorf("could not retrieve user with id %d", id)
	}

	return &user, nil
}

func (psqlDao *PSQLDao) GetAllUsers() ([]User, error) {
	users := []User{}
	query := "SELECT * FROM usr"
	err := psqlDao.DBConnection.Select(&users, query)

	if err != nil {
		logrus.Errorf("Could not retrieve all user, failed with error %s", err)
		return nil, fmt.Errorf("could not retrieve all users")
	}

	return users, nil
}

func (psqlDao *PSQLDao) CreateUser(name string, email string) (int64, error) {
	userMap := map[string]interface{}{
		"name":  name,
		"email": email,
	}
	stmt, err := psqlDao.DBConnection.PrepareNamed(`INSERT into usr (name,email) VALUES (:name,:email) RETURNING id`)
	if err != nil {
		logrus.Errorf("Could not prepare insert statement for user creation, failed with error %s", err)
		return 0, fmt.Errorf("could not prepare named statement for user creation")
	}

	var userId int64
	err = stmt.Get(&userId, userMap)
	if err != nil {
		logrus.Errorf("Could not create user, failed with error %s", err)
		return 0, fmt.Errorf("could not create user")
	}

	logrus.Infof("Sucessfully created user with id %d", userId)

	return userId, nil
}

func (psqlDao *PSQLDao) UpdateUser(user *User) error {
	userMap := map[string]interface{}{
		"name":  user.Name,
		"email": user.Email,
		"id":    user.ID,
	}
	_, err := psqlDao.DBConnection.NamedExec(`UPDATE usr SET name=:name, email=:email WHERE id=:id`, userMap)
	if err != nil {
		logrus.Errorf("Update user with id %d failed with error %s", user.ID, err)
		return fmt.Errorf("update user failed")
	}

	logrus.Infof("User %d updated successfully", user.ID)
	return nil
}

func deleteHelper(table string, id int64, psqlDao *PSQLDao, query string) error {
	userMap := map[string]interface{}{
		"id": id,
	}

	_, err := psqlDao.DBConnection.NamedExec(query, userMap)
	if err != nil {
		logrus.Errorf("Delete from table %s with id %d failed with error %s", table, id, err)
		return fmt.Errorf("delete from table %s failed", table)
	}

	logrus.Infof("object with id %d from table %s deleted sucessfully", id, table)
	return nil
}

func (psqlDao *PSQLDao) DeleteUser(id int64) error {
	table := "usr"
	query := fmt.Sprintf(`DELETE FROM %s WHERE id=:id`, table)
	return deleteHelper(table, id, psqlDao, query)
}

func (psqlDao *PSQLDao) GetWorkflowById(id int64) (*Workflow, error) {
	workflow := Workflow{}
	query := fmt.Sprintf("SELECT * FROM workflow WHERE wf_id=%d", id)
	err := psqlDao.DBConnection.Get(&workflow, query)

	if err != nil {
		logrus.Errorf("Could not retrieve workflow with id %d, failed with error %s", id, err)
		return nil, fmt.Errorf("could not get workflow")
	}

	return &workflow, nil
}

func (psqlDao *PSQLDao) GetAllWorkflows() ([]Workflow, error) {
	workflows := []Workflow{}
	query := "SELECT * FROM workflow"
	err := psqlDao.DBConnection.Select(&workflows, query)

	if err != nil {
		logrus.Errorf("Could not retrieve all user, failed with error %s", err)
		return nil, fmt.Errorf("could not retrieve all user")
	}

	return workflows, nil
}

func (psqlDao *PSQLDao) CreateWorkflow(userId int64, lastTaskCompleted int64, definition string, hash string, stats string, inputs JsonBytesMap, outputs string, status string, metadata JsonBytesMap) (int64, error) {
	workflowMap := map[string]interface{}{
		"usr_id":              userId,
		"last_task_completed": lastTaskCompleted,
		"definition":          definition,
		"hash":                hash,
		"stats":               stats,
		"inputs":              inputs,
		"outputs":             outputs,
		"status":              status,
		"metadata":            metadata,
	}

	var columns []string
	var columnValues []string
	for column := range workflowMap {
		columns = append(columns, column)
		columnValues = append(columnValues, fmt.Sprintf(":%s", column))
	}

	query := fmt.Sprintf(`INSERT into workflow (%s) VALUES (%s) RETURNING wf_id`, strings.Join(columns, ","), strings.Join(columnValues, ","))
	stmt, err := psqlDao.DBConnection.PrepareNamed(query)
	if err != nil {
		logrus.Errorf("Could not prepare insert statement for workflow creation, failed with error %s", err)
		return 0, fmt.Errorf("")
	}

	var workflowId int64
	err = stmt.Get(&workflowId, workflowMap)
	if err != nil {
		logrus.Errorf("Could not create workflow, failed with error %s", err)
		return 0, fmt.Errorf("could not create workflow")
	}

	return workflowId, nil
}

func (psqlDao *PSQLDao) UpdateWorkflow(workflow *Workflow) error {
	workflowMap := map[string]interface{}{
		"usr_id":              workflow.UserId,
		"last_task_completed": workflow.LastTaskCompleted,
		"definition":          workflow.Definition,
		"hash":                workflow.Hash,
		"stats":               workflow.Stats,
		"inputs":              workflow.Inputs,
		"outputs":             workflow.Outputs,
		"status":              workflow.Status,
		"metadata":            workflow.Metadata,
		"wf_id":               workflow.WorkFlowID,
	}

	var keyMapPairs []string
	for column := range workflowMap {
		if column == "wf_id" {
			continue
		}
		keyMapPairs = append(keyMapPairs, fmt.Sprintf("%s=:%s", column, column))
	}

	query := fmt.Sprintf(`UPDATE workflow SET %s WHERE wf_id=:wf_id`, strings.Join(keyMapPairs, ","))
	_, err := psqlDao.DBConnection.NamedExec(query, workflowMap)
	if err != nil {
		logrus.Errorf("Update workflow with id %d failed with error %s", workflow.WorkFlowID, err)
		return fmt.Errorf("update workflow failed")
	}

	logrus.Infof("Workflow with id %d updated successfully", workflow.WorkFlowID)
	return nil
}

func (psqlDao *PSQLDao) DeleteWorkflow(id int64) error {
	table := "workflow"
	query := fmt.Sprintf(`DELETE FROM %s WHERE wf_id=:id`, table)
	return deleteHelper(table, id, psqlDao, query)
}

func (psqlDao *PSQLDao) GetTaskById(id int64) (*Task, error) {
	task := Task{}
	query := fmt.Sprintf("SELECT * FROM task WHERE task_id=%d", id)
	err := psqlDao.DBConnection.Get(&task, query)

	if err != nil {
		logrus.Errorf("Could not retrieve task with id %d, failed with error %s", id, err)
		return nil, fmt.Errorf("could not retrieve task")
	}

	return &task, nil
}

func (psqlDao *PSQLDao) GetAllTasks() ([]Task, error) {
	tasks := []Task{}
	query := "SELECT * FROM usr"
	err := psqlDao.DBConnection.Select(&tasks, query)

	if err != nil {
		logrus.Errorf("Could not retrieve all tasks, failed with error %s", err)
		return nil, fmt.Errorf("retireve all tasks failed")
	}

	return tasks, nil
}

func (psqlDao *PSQLDao) CreateTask(wf_id int64, name string, hash string, stats string, input JsonBytesMap, output string, status string, taskError string, wf_status string) (int64, error) {
	taskMap := map[string]interface{}{
		"wf_id":     wf_id,
		"name":      name,
		"hash":      hash,
		"stats":     stats,
		"input":     input,
		"output":    output,
		"status":    status,
		"error":     taskError,
		"wf_status": wf_status,
	}

	var columns []string
	var columnValues []string
	for column := range taskMap {
		columns = append(columns, column)
		columnValues = append(columnValues, fmt.Sprintf(":%s", column))
	}

	query := fmt.Sprintf(`INSERT into task (%s) VALUES (%s) RETURNING task_id`, strings.Join(columns, ","), strings.Join(columnValues, ","))
	stmt, err := psqlDao.DBConnection.PrepareNamed(query)
	if err != nil {
		logrus.Errorf("Could not prepare insert statement for task creation, failed with error %s", err)
		return 0, fmt.Errorf("could not prepare named statement for task insert")
	}

	var taskId int64
	err = stmt.Get(&taskId, taskMap)
	if err != nil {
		logrus.Errorf("Could not create task with error %s", err)
		return 0, fmt.Errorf("could not create task")
	}

	return taskId, nil
}

func (psqlDao *PSQLDao) UpdateTask(task *Task) error {
	taskMap := map[string]interface{}{
		"wf_id":     task.WorkFlowID,
		"name":      task.Name,
		"hash":      task.Hash,
		"stats":     task.Stats,
		"input":     task.Input,
		"output":    task.Output,
		"status":    task.Status,
		"error":     task.Error,
		"wf_status": task.WorkFlowStatus,
		"task_id":   task.TaskId,
	}
	var keyMapPairs []string
	for column := range taskMap {
		if column == "task_id" {
			continue
		}
		keyMapPairs = append(keyMapPairs, fmt.Sprintf("%s=:%s", column, column))
	}

	query := fmt.Sprintf(`UPDATE task SET %s WHERE task_id=:wf_id`, strings.Join(keyMapPairs, ","))
	_, err := psqlDao.DBConnection.NamedExec(query, taskMap)
	if err != nil {
		logrus.Errorf("Update task with id %d failed with error %s", task.TaskId, err)
		return fmt.Errorf("update task failed")
	}

	logrus.Infof("task with id %d updated successfully", task.TaskId)
	return nil
}

func (psqlDao *PSQLDao) DeleteTask(id int64) error {
	table := "task"
	query := fmt.Sprintf(`DELETE FROM %s WHERE task_id=:id`, table)
	return deleteHelper(table, id, psqlDao, query)
}

func (psqlDao *PSQLDao) KillDao() {
	psqlDao.DBConnection.Close()
}
