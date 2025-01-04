# Gitchecker
Gitcheker is a simple script that is responsible for verifying .git repositories using a txt file that we provide. Once it finds a repository (https://example.com/.git/) it will save that line in another txt called findgitters.
## Install
`go build -o gitchecker main.go`
## Use
`./gitchecker <file.txt>`
