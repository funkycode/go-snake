default:
  just --list

build:
  @echo "Building..."
  go build -o bin/snake .
  @echo "Done"

run:
  go run .
