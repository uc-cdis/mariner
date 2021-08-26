_default: bin/database
	@: #random

bin/database: database/*.go
	go build -o bin/database
