package database

type Dao interface {
	GetAllUsers() ([]User, error)
	CreateUser(name string, email string) (int64, error)
	UpdateUser(user *User) error
	DeleteUser(id int64) error
	GetUserById(id int64) (*User, error)

	GetAllWorkflows() ([]Workflow, error)
	CreateWorkflow(userId int64, lastTaskCompleted int64, definition string, hash string, stats string, inputs JsonBytesMap, output string, status string, metadata JsonBytesMap) (int64, error)
	UpdateWorkflow(workflow *Workflow) error
	DeleteWorkflow(id int64) error
	GetWorkflowById(id int64) (*Workflow, error)

	GetAllTasks() ([]Task, error)
	CreateTask(wf_id int64, name string, hash string, stats string, input JsonBytesMap, output string, status string, taskError string, wf_status string) (int64, error)
	UpdateTask(task *Task) error
	DeleteTask(id int64) error
	GetTaskById(id int64) (*Task, error)

	KillDao()
}
