package database_test

import (
	"encoding/json"
	"errors"
	"log"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/uc-cdis/mariner/database"
)

func NewMock() (*sqlx.DB, sqlmock.Sqlmock) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		log.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}

	sqlxDB := sqlx.NewDb(mockDB, "sqlmock")

	return sqlxDB, mock
}

func TestGetById(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	userRows := sqlmock.NewRows([]string{"id", "name", "email", "created_at"}).
		AddRow(1, "name", "email", time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM usr WHERE id=1")).WillReturnRows(userRows)

	inputJson := map[string]interface{}{
		"inputkey": "inputval",
	}
	inputMarshalled, _ := json.Marshal(inputJson)
	taskRows := sqlmock.NewRows([]string{"task_id", "wf_id", "name", "hash", "stats", "input", "output", "attempt", "status", "return_code", "error", "wf_status", "created_at", "updated_at"}).
		AddRow(1, 1, "task name", "task hash", "task stats", inputMarshalled, "task output", 1, "task status", 1, "task error", "wf status", time.Now(), time.Now())
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM task WHERE task_id=1")).WillReturnRows(taskRows)

	inputsJson := map[string]interface{}{
		"inputskey": "inputsval",
	}
	inputsMarshalled, _ := json.Marshal(inputsJson)
	metadataJson := map[string]interface{}{
		"metadatakey": "metadataval",
	}
	metadataMarshalled, _ := json.Marshal(metadataJson)
	workflowRows := sqlmock.NewRows([]string{"wf_id", "usr_id", "last_task_completed", "definition", "hash", "stats", "inputs", "outputs", "status", "started_at", "ended_at", "created_at", "updated_at", "metadata"}).
		AddRow(1, 1, 1, "task definition", "task hash", "task stats", inputsMarshalled, "task outputs", "task status", time.Now(), time.Now(), time.Now(), time.Now(), metadataMarshalled)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM workflow WHERE wf_id=1")).WillReturnRows(workflowRows)

	userRes, _ := psqlDao.GetUserById(1)
	taskRes, _ := psqlDao.GetTaskById(1)
	workflowRes, _ := psqlDao.GetWorkflowById(1)
	assert.NotNil(t, userRes)
	assert.NotNil(t, taskRes)
	assert.NotNil(t, workflowRes)
}

func TestGetByIdFail(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM usr WHERE id=1")).WillReturnError(errors.New("could not get user"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM task WHERE task_id=1")).WillReturnError(errors.New("could not get task"))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM workflow WHERE wf_id=1")).WillReturnError(errors.New("could not get workflow"))

	userRes, _ := psqlDao.GetUserById(1)
	taskRes, _ := psqlDao.GetTaskById(1)
	workflowRes, _ := psqlDao.GetWorkflowById(1)
	assert.Nil(t, userRes)
	assert.Nil(t, taskRes)
	assert.Nil(t, workflowRes)
}

func TestCreate(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	userRows := sqlmock.NewRows([]string{"id"}).AddRow(1)
	mock.ExpectPrepare("INSERT into usr").ExpectQuery().WillReturnRows(userRows)

	taskRows := sqlmock.NewRows([]string{"task_id"}).AddRow(1)
	mock.ExpectPrepare("INSERT into task").ExpectQuery().WillReturnRows(taskRows)

	workflowRows := sqlmock.NewRows([]string{"wf_id"}).AddRow(1)
	mock.ExpectPrepare("INSERT into task").ExpectQuery().WillReturnRows(workflowRows)

	userRes, _ := psqlDao.CreateUser("name", "email")
	assert.True(t, userRes != 0)
}

func TestCreateFail(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	mock.ExpectPrepare("INSERT into usr").ExpectQuery().WillReturnError(errors.New("could not create user"))

	userRes, _ := psqlDao.CreateUser("name", "email")
	assert.True(t, userRes == 0)
}

func TestUpdate(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	user := database.User{
		Name:      "name",
		Email:     "email",
		ID:        1,
		CreatedAt: time.Now(),
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE usr SET name=?, email=? WHERE id=?")).WillReturnResult(sqlmock.NewResult(1, 1))
	err := psqlDao.UpdateUser(&user)
	assert.Nil(t, err)
}

func TestUpdateFail(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	user := database.User{
		Name:      "name",
		Email:     "email",
		ID:        1,
		CreatedAt: time.Now(),
	}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE usr SET name=?, email=? WHERE id=?")).WillReturnError(errors.New("could not update user"))
	userRes := psqlDao.UpdateUser(&user)
	assert.NotNil(t, userRes)
}

func TestDelete(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM usr WHERE id=?")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM task WHERE task_id=?")).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM workflow WHERE wf_id=?")).WillReturnResult(sqlmock.NewResult(1, 1))

	userErr := psqlDao.DeleteUser(1)
	taskErr := psqlDao.DeleteTask(1)
	workflowErr := psqlDao.DeleteWorkflow(1)
	assert.Nil(t, userErr)
	assert.Nil(t, taskErr)
	assert.Nil(t, workflowErr)
}

func TestDeleteFail(t *testing.T) {
	mockdb, mock := NewMock()
	var psqlDao database.PSQLDao
	psqlDao.DBConnection = mockdb
	defer psqlDao.KillDao()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM usr WHERE id=?")).WillReturnError(errors.New("could not delete user"))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM task WHERE task_id=?")).WillReturnError(errors.New("could not delete task"))
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM task WHERE wf_id=?")).WillReturnError(errors.New("could not delete workflow"))

	userErr := psqlDao.DeleteUser(1)
	taskErr := psqlDao.DeleteTask(1)
	workflowErr := psqlDao.DeleteWorkflow(1)
	assert.NotNil(t, userErr)
	assert.NotNil(t, taskErr)
	assert.NotNil(t, workflowErr)
}
