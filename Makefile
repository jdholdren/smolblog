.PHONY: test example

test:
	go test ./...

example:
	go run . -manifest=./example/smolmanifest.json
