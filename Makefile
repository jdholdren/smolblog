.PHONY: test example

test:
	go test ./...

example:
	go run . -manifest=./example/smolmanifest.json -output=build

wget -P build -nv -nH -r -E localhost:4444
