module github.com/huantingwei/go

replace github.com/huantingwei/go => ./


replace github.com/huantingwei/go/tracker => ./tracker

go 1.15

require (
	github.com/gin-contrib/sessions v0.0.3
	github.com/gin-gonic/gin v1.6.3
	github.com/huantingwei/go/tracker v0.0.0-00010101000000-000000000000 // indirect
	go.mongodb.org/mongo-driver v1.4.1
)
