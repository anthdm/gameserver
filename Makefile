client:
	@go build -o bin/client game_client/main.go
	@./bin/client

server:
	@go build -o bin/server game_server/main.go
	@./bin/server
