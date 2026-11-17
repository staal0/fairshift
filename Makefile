.PHONY: build test run db demo

build:
	go build -o bin/fairshift ./cmd/fairshift

test:
	go test ./...

db:
	podman run -d --name fairshift-mysql -p 127.0.0.1:3306:3306 \
	  -e MYSQL_ROOT_PASSWORD=fairshift -e MYSQL_DATABASE=fairshift \
	  -e MYSQL_USER=fairshift -e MYSQL_PASSWORD=fairshift \
	  -v fairshift-mysql:/var/lib/mysql docker.io/library/mysql:8.4

demo: build
	./bin/fairshift seed -force -org testdata/example-daycare/org.yaml -plan testdata/example-daycare/known-good.yaml

run: build
	./bin/fairshift serve
