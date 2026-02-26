include .env
migrate:
	migrate -source file://database/migrations \
		-database ${DB_URL} up

rollback:
	migrate -source file://database/migrations \
		-database ${DB_URL} down
	
drop:
	migrate -source file://database/migrations \
		-database ${DB_URL} drop

migration:
	@read -p "Enter migration name: " name; \
		migrate create -ext sql -dir database/migrations $$name

sqlc:
	sqlc generate

dev:
	go run main.go

client-dev:
	cd client && npm run dev
