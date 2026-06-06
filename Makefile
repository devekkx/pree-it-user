SQLC  := sqlc
GOOSE := goose
PG_DSN   := "host=localhost user=preeit_admin password=$$(cat ../../preeit-infra/secrets/postgres_password.txt) dbname=preeit sslmode=disable"

.PHONY: sqlc-gen sqlc-vet migrate-create migrate-up migrate-down migrate-status

sqlc-gen:
	$(SQLC) generate

# sqlc vet catches query/schema drift at codegen time - run in CI.
sqlc-vet:
	$(SQLC) vet

migrate-create:
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=<migration_name>"; exit 1; fi
	goose -dir db/migrations create $(name) sql

migrate-up:
	$(GOOSE) -dir db/migrations postgres $(PG_DSN) up

migrate-down:
	$(GOOSE) -dir db/migrations postgres $(PG_DSN) down

migrate-status:
	$(GOOSE) -dir db/migrations postgres $(PG_DSN) status